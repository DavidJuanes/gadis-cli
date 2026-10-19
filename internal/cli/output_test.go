package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// The --json output contract: stdout holds only JSON, errors go to stderr as
// {"error": …} with exit code 1, and warnings never reach stdout.

func TestJSONErrorFromBackend(t *testing.T) {
	fake := gadistest.New(t)
	fake.Fail("POST", "/catalog/products/search", 500)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "search", "leche", "--json")

	assertJSONError(t, res)
}

func TestJSONErrorFromUsage(t *testing.T) {
	gadis := clitest.New(t, gadistest.New(t))

	res := gadis.Run(t, "search", "--json")

	assertJSONError(t, res)
}

func TestErrorWithoutJSONIsText(t *testing.T) {
	fake := gadistest.New(t)
	fake.Fail("POST", "/catalog/products/search", 500)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "search", "leche")

	if res.Code != 1 || res.Stdout != "" || !strings.HasPrefix(res.Stderr, "error: ") {
		t.Errorf("code %d, stdout %q, stderr %q; want code 1 and \"error: …\" on stderr", res.Code, res.Stdout, res.Stderr)
	}
}

func TestWarningsDoNotPolluteJSON(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)
	fake.Fail("GET", "/api/auth/session", 500) // the account's postal code can't be read

	res := gadis.Run(t, "search", "leche", "--json").Success(t)

	var out struct {
		Store struct {
			Source string `json:"source"`
		} `json:"store"`
	}
	res.JSON(t, &out)
	if out.Store.Source != "default" {
		t.Errorf("store source = %q, want the default store", out.Store.Source)
	}
	if !strings.Contains(res.Stderr, "aviso:") {
		t.Errorf("stderr = %q, want a warning", res.Stderr)
	}
}

func assertJSONError(t *testing.T, res clitest.Result) {
	t.Helper()
	if res.Code != 1 {
		t.Errorf("exit code %d, want 1", res.Code)
	}
	if res.Stdout != "" {
		t.Errorf("stdout = %q, want nothing", res.Stdout)
	}
	var e map[string]string
	if err := json.Unmarshal([]byte(res.Stderr), &e); err != nil || e["error"] == "" || len(e) != 1 {
		t.Errorf("stderr = %q, want {\"error\": …}", res.Stderr)
	}
}
