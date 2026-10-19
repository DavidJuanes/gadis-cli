package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// Login and session: `gadis login`, the session it leaves on disk, and the
// commands that use it, run end to end against the fake Gadisline.

func TestLoginCreatesSessionReusedByLaterRuns(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "login", "--user", "ana@example.com", "--password", "s3cret").Success(t)

	if !strings.Contains(res.Stdout, "ana@example.com") {
		t.Errorf("stdout = %q, want the account's email", res.Stdout)
	}
	if fake.Session.CallbackMethod != "GET" {
		t.Errorf("NextAuth callback followed with %q, want GET", fake.Session.CallbackMethod)
	}
	assertNoFileContains(t, gadis.Dir, "s3cret")

	var who struct {
		Authenticated bool   `json:"authenticated"`
		Email         string `json:"email"`
	}
	gadis.Run(t, "whoami", "--json").Success(t).JSON(t, &who)
	if !who.Authenticated || who.Email != "ana@example.com" {
		t.Errorf("whoami = %+v, want the session of ana@example.com", who)
	}
}

func TestRotatedSessionCookiesAreSavedPrivately(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)

	// Each whoami asks NextAuth for a token, which rotates the cookies; a
	// later run only works if the previous one saved them.
	gadis.Run(t, "whoami").Success(t)
	gadis.Run(t, "whoami").Success(t)

	path := filepath.Join(gadis.Dir, "session.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file permissions = %o, want 600", perm)
	}
	var cookies []struct{ Name, Value string }
	readJSONFile(t, path, &cookies)
	v0, v1 := fake.SessionCookies()
	want := map[string]string{gadistest.SessionCookie + ".0": v0, gadistest.SessionCookie + ".1": v1}
	for _, c := range cookies {
		if w, ok := want[c.Name]; ok {
			if c.Value != w {
				t.Errorf("saved %s = %q, want the latest value %q", c.Name, c.Value, w)
			}
			delete(want, c.Name)
		}
	}
	if len(want) > 0 {
		t.Errorf("session file lacks %v", want)
	}
	if v0 == gadistest.SessionValue0 {
		t.Error("the fake never rotated the session cookies")
	}
}

func TestLoginWithWrongPasswordShowsKeycloakMessage(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "login", "--user", "ana@example.com", "--password", "wrong")

	if res.Code != 1 || !strings.Contains(res.Stderr, "Usuario o contraseña incorrectos.") {
		t.Errorf("code %d, stderr %q; want Keycloak's message", res.Code, res.Stderr)
	}
	assertNoSession(t, gadis)
}

func TestLoginAsksForCodeOnTerminal(t *testing.T) {
	fake := gadistest.New(t)
	fake.Session.OTP = true
	gadis := clitest.New(t, fake)
	gadis.Terminal = true
	gadis.Stdin = "123456\n"

	res := gadis.Run(t, "login", "--user", "ana@example.com", "--password", "s3cret").Success(t)

	if !strings.Contains(res.Stderr, "Código de verificación") {
		t.Errorf("stderr = %q, want a prompt for the code", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Introduce el código que te hemos enviado por SMS.") {
		t.Errorf("stderr = %q, want Keycloak's message", res.Stderr)
	}
	if got := fake.Session.Codes; len(got) != 1 || got[0] != "123456" {
		t.Errorf("Keycloak got codes %q, want [123456]", got)
	}
	gadis.Run(t, "whoami").Success(t)
}

func TestLoginGivesUpAfterThreeWrongCodes(t *testing.T) {
	fake := gadistest.New(t)
	fake.Session.OTP = true
	gadis := clitest.New(t, fake)
	gadis.Terminal = true
	gadis.Stdin = "111111\n222222\n333333\n123456\n"

	res := gadis.Run(t, "login", "--user", "ana@example.com", "--password", "s3cret")

	if res.Code != 1 || !strings.Contains(res.Stderr, "demasiados intentos") {
		t.Errorf("code %d, stderr %q; want too many attempts", res.Code, res.Stderr)
	}
	if got := fake.Session.Codes; len(got) != 3 {
		t.Errorf("Keycloak got codes %q, want the three wrong ones only", got)
	}
	assertNoSession(t, gadis)
}

func TestLoginWithCodeWithoutTerminalSuggestsBrowser(t *testing.T) {
	fake := gadistest.New(t)
	fake.Session.OTP = true
	gadis := clitest.New(t, fake)
	gadis.Stdin = "123456\n" // stdin is not a terminal: it must not be read as the code

	res := gadis.Run(t, "login", "--user", "ana@example.com", "--password", "s3cret")

	if res.Code != 1 || !strings.Contains(res.Stderr, "--browser") {
		t.Errorf("code %d, stderr %q; want a failure suggesting --browser", res.Code, res.Stderr)
	}
	if len(fake.Session.Codes) != 0 {
		t.Errorf("Keycloak got codes %q, want none", fake.Session.Codes)
	}
	assertNoSession(t, gadis)
}

func TestLoginCredentialPrecedence(t *testing.T) {
	flags := []string{"--user", "flag@example.com", "--password", "flag-pass"}
	env := map[string]string{"GADIS_EMAIL": "env@example.com", "GADIS_PASSWORD": "env-pass"}
	terminal := "term@example.com\nterm-pass\n"
	tests := []struct {
		name  string
		flags []string
		env   map[string]string
		stdin string
		want  gadistest.Credentials
	}{
		{"flags over environment and terminal", flags, env, terminal, gadistest.Credentials{Username: "flag@example.com", Password: "flag-pass"}},
		{"environment over terminal", nil, env, terminal, gadistest.Credentials{Username: "env@example.com", Password: "env-pass"}},
		{"terminal", nil, nil, terminal, gadistest.Credentials{Username: "term@example.com", Password: "term-pass"}},
		{"each value on its own", []string{"--user", "flag@example.com"}, map[string]string{"GADIS_PASSWORD": "env-pass"}, terminal,
			gadistest.Credentials{Username: "flag@example.com", Password: "env-pass"}},
		{"only the missing value from the terminal", nil, map[string]string{"GADIS_EMAIL": "env@example.com"}, "term-pass\n",
			gadistest.Credentials{Username: "env@example.com", Password: "term-pass"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := gadistest.New(t)
			gadis := clitest.New(t, fake)
			gadis.Terminal = true
			gadis.Stdin = tt.stdin
			for k, v := range tt.env {
				gadis.Env[k] = v
			}

			gadis.Run(t, append([]string{"login"}, tt.flags...)...) // the fake rejects these credentials

			if got := fake.Session.Logins; len(got) != 1 || got[0] != tt.want {
				t.Errorf("Keycloak got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoginWithoutCredentialsOrTerminalFails(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Stdin = "ana@example.com\ns3cret\n"

	res := gadis.Run(t, "login")

	if res.Code != 1 || !strings.Contains(res.Stderr, "GADIS_PASSWORD") {
		t.Errorf("code %d, stderr %q; want a failure naming the ways to give credentials", res.Code, res.Stderr)
	}
	if len(fake.Session.Logins) != 0 {
		t.Errorf("Keycloak got %+v, want nothing", fake.Session.Logins)
	}
}

func TestLogoutForgetsSession(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)

	gadis.Run(t, "logout").Success(t)

	assertNoSession(t, gadis)
	data, err := os.ReadFile(filepath.Join(gadis.Dir, "session.json"))
	if err == nil && strings.Contains(string(data), gadistest.SessionCookie) {
		t.Errorf("session file still holds the session cookies:\n%s", data)
	}
}

func TestWhoamiShowsAccountAndDefaultDelivery(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)

	res := gadis.Run(t, "whoami").Success(t)

	for _, want := range []string{"ana@example.com", "Ana Pérez", "CP 15002", "a domicilio", "El carrito está asociado al CP 15001"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, res.Stdout)
		}
	}
	reqs := fake.Requests("GET", "/clients/client-1/default-delivery")
	if len(reqs) != 1 || !strings.HasPrefix(reqs[0].Header.Get("Authorization"), "Bearer ") {
		t.Errorf("default-delivery got %d requests, want one with a Bearer token", len(reqs))
	}
}

func TestWhoamiDoesNotWarnWhenCartIsInDefaultPostalCode(t *testing.T) {
	fake := gadistest.New(t)
	fake.Cart.PostalCode = "15002"
	gadis := clitest.New(t, fake)
	gadis.Login(t)

	res := gadis.Run(t, "whoami").Success(t)

	if strings.Contains(res.Stdout, "carrito") {
		t.Errorf("stdout = %q, want no cart warning", res.Stdout)
	}
}

func TestWhoamiJSON(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)

	var out struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Delivery struct {
			PostalCode string `json:"postal_code"`
			StoreID    string `json:"store_id"`
		} `json:"default_delivery"`
		CartPostalCode string `json:"cart_postal_code"`
	}
	gadis.Run(t, "whoami", "--json").Success(t).JSON(t, &out)

	if out.Email != "ana@example.com" || out.Name != "Ana Pérez" || out.Delivery.PostalCode != "15002" ||
		out.Delivery.StoreID != "store-2" || out.CartPostalCode != "15001" {
		t.Errorf("whoami = %+v", out)
	}
}

func TestCommandsNeedingSessionFailWithoutIt(t *testing.T) {
	for _, args := range [][]string{{"whoami"}, {"cart"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			gadis := clitest.New(t, gadistest.New(t))

			res := gadis.Run(t, args...)

			if res.Code != 1 || !strings.Contains(res.Stderr, "gadis login") {
				t.Errorf("code %d, stderr %q; want a failure pointing to `gadis login`", res.Code, res.Stderr)
			}
		})
	}
}

// assertNoSession fails unless the CLI has no usable session.
func assertNoSession(t *testing.T, gadis *clitest.CLI) {
	t.Helper()
	res := gadis.Run(t, "whoami")
	if res.Code == 0 {
		t.Errorf("whoami succeeded, want no session:\n%s", res.Stdout)
	}
}

func readJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// assertNoFileContains fails if any file under dir contains secret.
func assertNoFileContains(t *testing.T, dir, secret string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), secret) {
			t.Errorf("%s contains the password", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
