package gadis_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/gadis"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

func TestLoginSuccess(t *testing.T) {
	fake := gadistest.New(t)
	c := fake.Client()
	ch, err := c.Login(context.Background(), "ana@example.com", "s3cret")
	if err != nil || ch != nil {
		t.Fatalf("Login = %v, %v; want success", ch, err)
	}
	if fake.Session.CallbackMethod != http.MethodGet {
		t.Errorf("NextAuth callback followed with %s, want GET", fake.Session.CallbackMethod)
	}
	if !c.LoggedIn() {
		t.Fatal("no session cookie after login")
	}
	acc, err := c.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acc.ClientID != "client-1" || acc.Name != "Ana Pérez" || acc.Cart.ID != "cart-1" || len(acc.Cart.Lines) != 1 {
		t.Errorf("unexpected account %+v", acc)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	c := gadistest.New(t).Client()
	_, err := c.Login(context.Background(), "ana@example.com", "nope")
	var le *gadis.LoginError
	if !errors.As(err, &le) || le.Message != "Usuario o contraseña incorrectos." {
		t.Fatalf("err = %v, want LoginError with Keycloak's message", err)
	}
	if c.LoggedIn() {
		t.Error("logged in with a wrong password")
	}
}

func TestLoginWithCode(t *testing.T) {
	fake := gadistest.New(t)
	fake.Session.OTP = true
	c := fake.Client()
	ch, err := c.Login(context.Background(), "ana@example.com", "s3cret")
	if !errors.Is(err, gadis.ErrCodeRequired) || ch == nil {
		t.Fatalf("Login = %v, %v; want a code challenge", ch, err)
	}
	if ch.Field != "otp" || !strings.Contains(ch.Message, "SMS") {
		t.Errorf("challenge = %+v", ch)
	}

	ch, err = c.SubmitCode(context.Background(), ch, "000000")
	if !errors.Is(err, gadis.ErrCodeRequired) || ch.Message != "Código no válido." {
		t.Fatalf("wrong code: %v, %+v", err, ch)
	}
	ch, err = c.SubmitCode(context.Background(), ch, " 123456 ")
	if err != nil || ch != nil {
		t.Fatalf("SubmitCode = %v, %v; want success", ch, err)
	}
	if !c.LoggedIn() {
		t.Error("no session after the code")
	}
}

func TestAccountWithoutSession(t *testing.T) {
	c := gadistest.New(t).Client()
	if _, err := c.Account(context.Background()); !errors.Is(err, gadis.ErrNotLoggedIn) {
		t.Errorf("err = %v, want ErrNotLoggedIn", err)
	}
}
