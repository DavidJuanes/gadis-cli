package gadis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGadisline mimics the NextAuth + Keycloak login and the home page.
// With otp set, Keycloak asks for a code after the password.
type fakeGadisline struct {
	otp       bool
	password  string
	gotMethod string // method used on the NextAuth callback
}

func (f *fakeGadisline) handler(base func() string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/csrf", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "__Host-next-auth.csrf-token", Value: "csrf|hash", Path: "/", Secure: true})
		fmt.Fprint(w, `{"csrfToken":"csrf"}`)
	})
	mux.HandleFunc("POST /api/auth/signin/keycloak", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("csrfToken") != "csrf" || r.FormValue("json") != "true" {
			http.Error(w, "bad signin", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"url": base() + "/realms/gadisline/protocol/openid-connect/auth?client_id=gadisline"})
	})
	mux.HandleFunc("GET /realms/gadisline/protocol/openid-connect/auth", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "AUTH_SESSION_ID", Value: "kc", Path: "/realms/gadisline/"})
		fmt.Fprint(w, `<html><body><form id="loginForm" action="/realms/gadisline/login-actions/authenticate?session_code=x&amp;execution=y" method="post">
			<input name="username" type="text"><input name="password" type="password">
			<input type="hidden" name="credentialId" value=""></form></body></html>`)
	})
	mux.HandleFunc("POST /realms/gadisline/login-actions/authenticate", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session_code") != "x" {
			http.Error(w, "lost query", 400)
			return
		}
		if r.FormValue("otp") != "" {
			if r.FormValue("otp") != "123456" || r.FormValue("hidden") != "keep" {
				fmt.Fprint(w, `<form action="/realms/gadisline/login-actions/authenticate?session_code=x"><input name="otp"><input type="hidden" name="hidden" value="keep"></form><span id="input-error-otp">Código no válido.</span>`)
				return
			}
			http.Redirect(w, r, "/api/auth/callback/keycloak?code=abc", http.StatusFound)
			return
		}
		if r.FormValue("username") != "ana@example.com" || r.FormValue("password") != f.password {
			fmt.Fprint(w, `<form id="loginForm" action="/x"><input name="username"><input name="password" type="password"></form>
				<span id="input-error" class="kc-feedback-text">Usuario o contraseña incorrectos.</span>`)
			return
		}
		if f.otp {
			fmt.Fprint(w, `<form id="kc-otp-login-form" action="/realms/gadisline/login-actions/authenticate?session_code=x"><input name="otp" type="text"><input type="hidden" name="hidden" value="keep"></form><p class="kc-feedback-text">Introduce el código que te hemos enviado por SMS.</p>`)
			return
		}
		http.Redirect(w, r, "/api/auth/callback/keycloak?code=abc", http.StatusFound)
	})
	mux.HandleFunc("/api/auth/callback/keycloak", func(w http.ResponseWriter, r *http.Request) {
		f.gotMethod = r.Method
		if r.Method != http.MethodGet {
			http.Redirect(w, r, "/api/auth/error", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie + ".0", Value: "jwe-part-0", Path: "/", Secure: true, HttpOnly: true, MaxAge: 2592000})
		http.SetCookie(w, &http.Cookie{Name: sessionCookie + ".1", Value: "jwe-part-1", Path: "/", Secure: true, HttpOnly: true, MaxAge: 2592000})
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("GET /api/auth/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "This action with HTTP POST is not supported by NextAuth.js", 400)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		props := `{"clientData":null,"cart":null}`
		if c, err := r.Cookie(sessionCookie + ".0"); err == nil && c.Value == "jwe-part-0" {
			props = `{"clientData":{"id":"client-1","email":"ana@example.com","given_name":"Ana","family_name":"Pérez","postal_code":"15002","phone_verified":true},
				"cart":{"id":"cart-1","store_id":"store-1","postal_code":"15001","products":[{"product_id":"p1","product_name":"Yogur","amount":2,"line_price":1.58}],"total_cart_price":6.08}}`
		}
		fmt.Fprintf(w, `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":%s}}</script></html>`, props)
	})
	return mux
}

func newFakeClient(t *testing.T, f *fakeGadisline) *Client {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(f.handler(func() string { return srv.URL }))
	t.Cleanup(srv.Close)
	c := New(&Jar{})
	c.HTTP.Transport = srv.Client().Transport
	c.URLs = Endpoints{Web: srv.URL, Catalog: srv.URL, Store: srv.URL, Cart: srv.URL, Clients: srv.URL}
	return c
}

func TestLoginSuccess(t *testing.T) {
	f := &fakeGadisline{password: "s3cret"}
	c := newFakeClient(t, f)
	ch, err := c.Login(context.Background(), "ana@example.com", "s3cret")
	if err != nil || ch != nil {
		t.Fatalf("Login = %v, %v; want success", ch, err)
	}
	if f.gotMethod != http.MethodGet {
		t.Errorf("NextAuth callback followed with %s, want GET", f.gotMethod)
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
	c := newFakeClient(t, &fakeGadisline{password: "s3cret"})
	_, err := c.Login(context.Background(), "ana@example.com", "nope")
	var le *LoginError
	if !errors.As(err, &le) || le.Message != "Usuario o contraseña incorrectos." {
		t.Fatalf("err = %v, want LoginError with Keycloak's message", err)
	}
	if c.LoggedIn() {
		t.Error("logged in with a wrong password")
	}
}

func TestLoginWithCode(t *testing.T) {
	c := newFakeClient(t, &fakeGadisline{password: "s3cret", otp: true})
	ch, err := c.Login(context.Background(), "ana@example.com", "s3cret")
	if !errors.Is(err, ErrCodeRequired) || ch == nil {
		t.Fatalf("Login = %v, %v; want a code challenge", ch, err)
	}
	if ch.Field != "otp" || !strings.Contains(ch.Message, "SMS") {
		t.Errorf("challenge = %+v", ch)
	}

	ch, err = c.SubmitCode(context.Background(), ch, "000000")
	if !errors.Is(err, ErrCodeRequired) || ch.Message != "Código no válido." {
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
	c := newFakeClient(t, &fakeGadisline{})
	if _, err := c.Account(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err = %v, want ErrNotLoggedIn", err)
	}
}
