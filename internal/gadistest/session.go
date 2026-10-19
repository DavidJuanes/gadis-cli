package gadistest

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Session cookie names and values of the fake NextAuth. The session cookie is
// split in two (.0/.1), as the real one is (~5 KB).
const (
	SessionCookie  = "__Secure-next-auth.session-token"
	SessionValue0  = "jwe-part-0"
	SessionValue1  = "jwe-part-1"
	sessionMaxAge  = 2592000 // 30 days
	keycloakPrefix = "/realms/gadisline"
)

// Session is the login area: NextAuth, Keycloak and the account shown on the
// home page once logged in.
type Session struct {
	Email    string
	Password string
	OTP      bool   // Keycloak asks for a code after the password
	Code     string // the right code when OTP is set

	ClientID   string
	GivenName  string
	FamilyName string
	PostalCode string // profile postal code in clientData (not the account's delivery one)

	// CallbackMethod is the method used on the last NextAuth callback.
	CallbackMethod string
}

func newSession() *Session {
	return &Session{
		Email: "ana@example.com", Password: "s3cret", Code: "123456",
		ClientID: "client-1", GivenName: "Ana", FamilyName: "Pérez", PostalCode: "15002",
	}
}

// loggedIn reports whether r carries the fake's session cookies.
func (f *Session) loggedIn(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie + ".0")
	return err == nil && c.Value == SessionValue0
}

func (f *Session) register(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /api/auth/csrf", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "__Host-next-auth.csrf-token", Value: "csrf|hash", Path: "/", Secure: true})
		fmt.Fprint(w, `{"csrfToken":"csrf"}`)
	})
	mux.HandleFunc("POST /api/auth/signin/keycloak", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("csrfToken") != "csrf" || r.FormValue("json") != "true" {
			http.Error(w, "bad signin", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"url": s.URL() + keycloakPrefix + "/protocol/openid-connect/auth?client_id=gadisline"})
	})
	mux.HandleFunc("GET "+keycloakPrefix+"/protocol/openid-connect/auth", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "AUTH_SESSION_ID", Value: "kc", Path: keycloakPrefix + "/"})
		fmt.Fprint(w, `<html><body><form id="loginForm" action="/realms/gadisline/login-actions/authenticate?session_code=x&amp;execution=y" method="post">
			<input name="username" type="text"><input name="password" type="password">
			<input type="hidden" name="credentialId" value=""></form></body></html>`)
	})
	mux.HandleFunc("POST "+keycloakPrefix+"/login-actions/authenticate", func(w http.ResponseWriter, r *http.Request) {
		defer s.lock()()
		if r.URL.Query().Get("session_code") != "x" {
			http.Error(w, "lost query", 400)
			return
		}
		if r.FormValue("otp") != "" {
			if r.FormValue("otp") != f.Code || r.FormValue("hidden") != "keep" {
				fmt.Fprint(w, `<form action="/realms/gadisline/login-actions/authenticate?session_code=x"><input name="otp"><input type="hidden" name="hidden" value="keep"></form><span id="input-error-otp">Código no válido.</span>`)
				return
			}
			http.Redirect(w, r, "/api/auth/callback/keycloak?code=abc", http.StatusFound)
			return
		}
		if r.FormValue("username") != f.Email || r.FormValue("password") != f.Password {
			fmt.Fprint(w, `<form id="loginForm" action="/x"><input name="username"><input name="password" type="password"></form>
				<span id="input-error" class="kc-feedback-text">Usuario o contraseña incorrectos.</span>`)
			return
		}
		if f.OTP {
			fmt.Fprint(w, `<form id="kc-otp-login-form" action="/realms/gadisline/login-actions/authenticate?session_code=x"><input name="otp" type="text"><input type="hidden" name="hidden" value="keep"></form><p class="kc-feedback-text">Introduce el código que te hemos enviado por SMS.</p>`)
			return
		}
		http.Redirect(w, r, "/api/auth/callback/keycloak?code=abc", http.StatusFound)
	})
	mux.HandleFunc("/api/auth/callback/keycloak", func(w http.ResponseWriter, r *http.Request) {
		unlock := s.lock()
		f.CallbackMethod = r.Method
		unlock()
		if r.Method != http.MethodGet {
			// NextAuth rejects the callback by POST.
			http.Redirect(w, r, "/api/auth/error", http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: SessionCookie + ".0", Value: SessionValue0, Path: "/", Secure: true, HttpOnly: true, MaxAge: sessionMaxAge})
		http.SetCookie(w, &http.Cookie{Name: SessionCookie + ".1", Value: SessionValue1, Path: "/", Secure: true, HttpOnly: true, MaxAge: sessionMaxAge})
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("GET /api/auth/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "This action with HTTP POST is not supported by NextAuth.js", 400)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		defer s.lock()()
		props := map[string]any{"clientData": nil, "cart": nil}
		if f.loggedIn(r) {
			props["clientData"] = map[string]any{
				"id": f.ClientID, "email": f.Email, "given_name": f.GivenName, "family_name": f.FamilyName,
				"postal_code": f.PostalCode, "phone_verified": true,
			}
			props["cart"] = s.Cart.nextData()
		}
		data, _ := json.Marshal(map[string]any{"props": map[string]any{"pageProps": props}})
		fmt.Fprintf(w, `<html><script id="__NEXT_DATA__" type="application/json">%s</script></html>`, data)
	})
}
