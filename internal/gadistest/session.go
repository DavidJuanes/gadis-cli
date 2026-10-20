package gadistest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
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

// Session is the login area: NextAuth, Keycloak, the account shown on the
// home page once logged in, and the account's default delivery (clients
// service), which needs the NextAuth access token.
type Session struct {
	Email    string
	Password string
	OTP      bool   // Keycloak asks for a code after the password
	Code     string // the right code when OTP is set

	ClientID   string
	GivenName  string
	FamilyName string
	PostalCode string // profile postal code in clientData (not the account's delivery one)

	// DefaultDelivery is the account's default delivery (clients service,
	// default-delivery). It is the account's own postal code, unlike the cart's.
	DefaultDelivery Delivery

	// CallbackMethod is the method used on the last NextAuth callback.
	CallbackMethod string
	// Logins are the username/password forms Keycloak received, in order.
	Logins []Credentials
	// Codes are the one-time codes Keycloak received, in order.
	Codes []string

	// rotation counts how many times /api/auth/session refreshed the token.
	// Like NextAuth, each refresh rewrites the session cookies; the fake
	// accepts only the latest ones, so a client that does not save them loses
	// the session.
	rotation int
}

// Delivery is the shape of clients/<id>/default-delivery.
type Delivery struct {
	PostalCode   string `json:"postal_code"`
	StoreID      string `json:"store_id"`
	DeliveryType string `json:"delivery_type"`
}

// Credentials are what Keycloak received on its login form.
type Credentials struct{ Username, Password string }

func newSession() *Session {
	return &Session{
		Email: "ana@example.com", Password: "s3cret", Code: "123456",
		ClientID: "client-1", GivenName: "Ana", FamilyName: "Pérez", PostalCode: "15002",
		DefaultDelivery: Delivery{PostalCode: "15002", StoreID: "store-2", DeliveryType: "HOME_DELIVERY"},
	}
}

// SessionCookies returns the current values of the two session cookies (.0 and .1).
// They change every time /api/auth/session refreshes the token.
func (s *Server) SessionCookies() (string, string) {
	defer s.lock()()
	return s.Session.cookieValues()
}

// ExpireSession invalidates the session the client holds, as if it had
// expired on the server: its cookies stop being accepted.
func (s *Server) ExpireSession() {
	defer s.lock()()
	s.Session.rotation += 1000
}

func (se *Session) cookieValues() (string, string) {
	if se.rotation == 0 {
		return SessionValue0, SessionValue1
	}
	n := "-r" + strconv.Itoa(se.rotation)
	return SessionValue0 + n, SessionValue1 + n
}

// accessToken is the Keycloak token issued on the current rotation.
func (se *Session) accessToken() string { return "access-token-" + strconv.Itoa(se.rotation) }

func (se *Session) setSessionCookies(w http.ResponseWriter) {
	v0, v1 := se.cookieValues()
	http.SetCookie(w, &http.Cookie{Name: SessionCookie + ".0", Value: v0, Path: "/", Secure: true, HttpOnly: true, MaxAge: sessionMaxAge})
	http.SetCookie(w, &http.Cookie{Name: SessionCookie + ".1", Value: v1, Path: "/", Secure: true, HttpOnly: true, MaxAge: sessionMaxAge})
}

// loggedIn reports whether r carries the fake's current session cookies.
// Callers hold the lock.
func (se *Session) loggedIn(r *http.Request) bool {
	v0, v1 := se.cookieValues()
	c0, err0 := r.Cookie(SessionCookie + ".0")
	c1, err1 := r.Cookie(SessionCookie + ".1")
	return err0 == nil && err1 == nil && c0.Value == v0 && c1.Value == v1
}

func (se *Session) register(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /api/auth/csrf", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "__Host-next-auth.csrf-token", Value: "csrf|hash", Path: "/", Secure: true})
		fmt.Fprint(w, `{"csrfToken":"csrf"}`)
	})
	mux.HandleFunc("POST /api/auth/signin/keycloak", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("csrfToken") != "csrf" || r.FormValue("json") != "true" {
			http.Error(w, "bad signin", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"url": s.URL() + keycloakPrefix + "/protocol/openid-connect/auth?client_id=gadisline"})
	})
	mux.HandleFunc("GET "+keycloakPrefix+"/protocol/openid-connect/auth", func(w http.ResponseWriter, _ *http.Request) {
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
			se.Codes = append(se.Codes, r.FormValue("otp"))
			if r.FormValue("otp") != se.Code || r.FormValue("hidden") != "keep" {
				fmt.Fprint(w, `<form action="/realms/gadisline/login-actions/authenticate?session_code=x"><input name="otp"><input type="hidden" name="hidden" value="keep"></form><span id="input-error-otp">Código no válido.</span>`)
				return
			}
			http.Redirect(w, r, "/api/auth/callback/keycloak?code=abc", http.StatusFound)
			return
		}
		se.Logins = append(se.Logins, Credentials{r.FormValue("username"), r.FormValue("password")})
		if r.FormValue("username") != se.Email || r.FormValue("password") != se.Password {
			fmt.Fprint(w, `<form id="loginForm" action="/x"><input name="username"><input name="password" type="password"></form>
				<span id="input-error" class="kc-feedback-text">Usuario o contraseña incorrectos.</span>`)
			return
		}
		if se.OTP {
			fmt.Fprint(w, `<form id="kc-otp-login-form" action="/realms/gadisline/login-actions/authenticate?session_code=x"><input name="otp" type="text"><input type="hidden" name="hidden" value="keep"></form><p class="kc-feedback-text">Introduce el código que te hemos enviado por SMS.</p>`)
			return
		}
		http.Redirect(w, r, "/api/auth/callback/keycloak?code=abc", http.StatusFound)
	})
	mux.HandleFunc("/api/auth/callback/keycloak", func(w http.ResponseWriter, r *http.Request) {
		defer s.lock()()
		se.CallbackMethod = r.Method
		if r.Method != http.MethodGet {
			// NextAuth rejects the callback by POST.
			http.Redirect(w, r, "/api/auth/error", http.StatusFound)
			return
		}
		se.setSessionCookies(w)
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("GET /api/auth/error", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "This action with HTTP POST is not supported by NextAuth.js", 400)
	})
	mux.HandleFunc("GET /api/auth/session", func(w http.ResponseWriter, r *http.Request) {
		defer s.lock()()
		if !se.loggedIn(r) {
			fmt.Fprint(w, `{}`) // NextAuth answers an empty object without a session
			return
		}
		// NextAuth refreshes the access token server-side and rewrites the
		// session cookies.
		se.rotation++
		se.setSessionCookies(w)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"expires": time.Now().Add(sessionMaxAge * time.Second).UTC().Format(time.RFC3339),
			"token":   map[string]string{"accessToken": se.accessToken()},
		})
	})
	mux.HandleFunc("POST /api/auth/signout", func(w http.ResponseWriter, _ *http.Request) {
		for _, name := range []string{SessionCookie + ".0", SessionCookie + ".1"} {
			http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", Secure: true, MaxAge: -1})
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"url": s.URL() + "/"})
	})
	mux.HandleFunc("GET "+clientsPrefix+"/clients/{id}/default-delivery", func(w http.ResponseWriter, r *http.Request) {
		if !requireSite(w, r) {
			return
		}
		defer s.lock()()
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token != se.accessToken() {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.PathValue("id") != se.ClientID {
			http.Error(w, `{"message":"Forbidden"}`, http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(se.DefaultDelivery) // an object, not a list
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		defer s.lock()()
		props := map[string]any{"clientData": nil, "cart": nil}
		if se.loggedIn(r) {
			props["clientData"] = map[string]any{
				"id": se.ClientID, "email": se.Email, "given_name": se.GivenName, "family_name": se.FamilyName,
				"postal_code": se.PostalCode, "phone_verified": true,
			}
			props["cart"] = s.Cart.nextData()
		}
		data, _ := json.Marshal(map[string]any{"props": map[string]any{"pageProps": props}})
		fmt.Fprintf(w, `<html><script id="__NEXT_DATA__" type="application/json">%s</script></html>`, data)
	})
}
