package gadis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// ErrNotLoggedIn means there is no valid gadisline.com session.
var ErrNotLoggedIn = errors.New("no hay sesión en Gadisline: ejecuta `gadis login`")

// ErrCodeRequired means Keycloak asked for a one-time code after the password.
var ErrCodeRequired = errors.New("Gadisline pide un código de verificación") //nolint:staticcheck // ST1005: "Gadisline" is a proper noun

// LoginChallenge is a Keycloak form that asks for a one-time code.
type LoginChallenge struct {
	Action  string     // form action
	Field   string     // name of the code input
	Hidden  url.Values // other inputs to send back
	Message string     // text shown by Keycloak, if any
}

// LoginError is a login rejected by Keycloak, with its message.
type LoginError struct{ Message string }

func (e *LoginError) Error() string {
	if e.Message == "" {
		return "el login ha fallado (Keycloak no dio motivo)"
	}
	return "el login ha fallado: " + e.Message
}

// LoggedIn reports whether the jar holds a NextAuth session cookie.
// It does not check that the session is still valid; Session does.
func (c *Client) LoggedIn() bool {
	host := c.webHost()
	return c.Jar.Has(host, sessionCookie) || c.Jar.Has(host, sessionCookie+".0")
}

// Login signs in with email and password through the same NextAuth→Keycloak
// flow the website uses, without a browser. Any previous session is discarded.
// If Keycloak asks for a one-time code it returns a challenge and
// ErrCodeRequired; pass the code to SubmitCode.
func (c *Client) Login(ctx context.Context, email, password string) (*LoginChallenge, error) {
	c.Jar.Clear()

	var csrf struct {
		Token string `json:"csrfToken"`
	}
	if err := c.do(ctx, request{method: "GET", url: c.URLs.Web + "/api/auth/csrf", out: &csrf}); err != nil {
		return nil, err
	}
	var signin struct {
		URL string `json:"url"`
	}
	err := c.do(ctx, request{
		method: "POST", url: c.URLs.Web + "/api/auth/signin/keycloak",
		body: url.Values{"csrfToken": {csrf.Token}, "callbackUrl": {c.URLs.Web + "/"}, "json": {"true"}},
		out:  &signin,
	})
	if err != nil {
		return nil, err
	}
	if signin.URL == "" {
		return nil, errors.New("NextAuth no devolvió la URL de Keycloak")
	}

	page, err := c.fetchPage(ctx, "GET", signin.URL, nil)
	if err != nil {
		return nil, err
	}
	form, ok := findForm(page.forms, func(f htmlForm) bool { return f.ID == "loginForm" || f.has("password") })
	if !ok {
		return nil, fmt.Errorf("no se encontró el formulario de login de Keycloak (%s)", page.url.Host)
	}
	values := form.values()
	values.Set("username", email)
	values.Set("password", password)
	values.Set("credentialId", "")
	page, err = c.fetchPage(ctx, "POST", form.Action, values)
	if err != nil {
		return nil, err
	}
	return c.loginOutcome(page)
}

// SubmitCode sends the one-time code Keycloak asked for.
func (c *Client) SubmitCode(ctx context.Context, ch *LoginChallenge, code string) (*LoginChallenge, error) {
	values := url.Values{}
	for k, v := range ch.Hidden {
		values[k] = v
	}
	values.Set(ch.Field, strings.TrimSpace(code))
	page, err := c.fetchPage(ctx, "POST", ch.Action, values)
	if err != nil {
		return nil, err
	}
	return c.loginOutcome(page)
}

var codeField = regexp.MustCompile(`(?i)^(otp|totp|code|sms[-_]?code|smscode|verification[-_]?code|emailcode)$`)

func (c *Client) loginOutcome(p *htmlPage) (*LoginChallenge, error) {
	if p.url.Hostname() == c.webHost() && c.LoggedIn() {
		return nil, nil
	}
	for _, f := range p.forms {
		for _, in := range f.Inputs {
			if codeField.MatchString(in.Name) {
				hidden := f.values()
				hidden.Del(in.Name)
				return &LoginChallenge{Action: f.Action, Field: in.Name, Hidden: hidden, Message: p.message}, ErrCodeRequired
			}
		}
	}
	if p.status >= 400 && p.message == "" {
		return nil, &LoginError{Message: fmt.Sprintf("HTTP %d en %s", p.status, p.url.Host+p.url.Path)}
	}
	return nil, &LoginError{Message: p.message}
}

// Logout signs out of NextAuth (best effort) and forgets the session.
func (c *Client) Logout(ctx context.Context) error {
	var csrf struct {
		Token string `json:"csrfToken"`
	}
	if err := c.do(ctx, request{method: "GET", url: c.URLs.Web + "/api/auth/csrf", out: &csrf}); err == nil {
		_ = c.do(ctx, request{
			method: "POST", url: c.URLs.Web + "/api/auth/signout",
			body: url.Values{"csrfToken": {csrf.Token}, "json": {"true"}},
		})
	}
	c.Jar.Clear()
	return c.Jar.Save()
}

// AccessToken returns a fresh Keycloak access token for the microservices.
// It lives ten minutes; NextAuth refreshes it server-side on each call.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if !c.LoggedIn() {
		return "", ErrNotLoggedIn
	}
	var s struct {
		Error string `json:"error"`
		Token struct {
			AccessToken string `json:"accessToken"`
			Error       string `json:"error"`
		} `json:"token"`
	}
	if err := c.do(ctx, request{method: "GET", url: c.URLs.Web + "/api/auth/session", out: &s}); err != nil {
		return "", err
	}
	if s.Token.AccessToken == "" || s.Error != "" || s.Token.Error != "" {
		return "", ErrNotLoggedIn
	}
	return s.Token.AccessToken, nil
}

// Account is the logged-in customer and their cart.
type Account struct {
	ClientID      string `json:"client_id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	PostalCode    string `json:"profile_postal_code"`
	PhoneVerified bool   `json:"phone_verified"`
	Cart          Cart   `json:"cart"`
}

// Account reads the customer and cart from the server-rendered home page.
func (c *Client) Account(ctx context.Context) (*Account, error) {
	if !c.LoggedIn() {
		return nil, ErrNotLoggedIn
	}
	var props struct {
		ClientData *struct {
			ID            string `json:"id"`
			Email         string `json:"email"`
			Name          string `json:"name"`
			GivenName     string `json:"given_name"`
			FamilyName    string `json:"family_name"`
			PostalCode    string `json:"postal_code"`
			PhoneVerified bool   `json:"phone_verified"`
		} `json:"clientData"`
		Cart *Cart `json:"cart"`
	}
	if err := c.pageProps(ctx, &props); err != nil {
		return nil, err
	}
	if props.ClientData == nil || props.ClientData.ID == "" {
		return nil, ErrNotLoggedIn
	}
	cd := props.ClientData
	name := strings.TrimSpace(cd.GivenName + " " + cd.FamilyName)
	if name == "" {
		name = cd.Name
	}
	a := &Account{ClientID: cd.ID, Email: cd.Email, Name: name, PostalCode: cd.PostalCode, PhoneVerified: cd.PhoneVerified}
	if props.Cart != nil {
		a.Cart = *props.Cart
	}
	return a, nil
}

// Delivery is the customer's default delivery: postal code, store and type.
type Delivery struct {
	PostalCode   string `json:"postal_code"`
	StoreID      string `json:"store_id"`
	DeliveryType string `json:"delivery_type"`
}

// DefaultDelivery returns the account's default delivery. This, and not the
// cart's postal code, is the account's own postal code: the website copies a
// postal code chosen before logging in onto the account's cart.
func (c *Client) DefaultDelivery(ctx context.Context, clientID string) (Delivery, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return Delivery{}, err
	}
	var out Delivery
	err = c.do(ctx, request{
		method: "GET", url: c.URLs.Clients + "/clients/" + url.PathEscape(clientID) + "/default-delivery",
		token: token, out: &out,
	})
	if err != nil {
		return Delivery{}, err
	}
	if out.PostalCode == "" {
		return Delivery{}, errors.New("la cuenta no tiene un código postal de reparto por defecto")
	}
	return out, nil
}

var nextData = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

// pageProps decodes props.pageProps of the server-rendered home page.
func (c *Client) pageProps(ctx context.Context, out any) error {
	page, err := c.fetchPage(ctx, "GET", c.URLs.Web+"/", nil)
	if err != nil {
		return err
	}
	if page.status != http.StatusOK {
		return &APIError{Method: "GET", URL: c.URLs.Web + "/", Status: page.status}
	}
	m := nextData.FindSubmatch(page.body)
	if m == nil {
		return errors.New("la portada de Gadisline no trae __NEXT_DATA__: puede que la web haya cambiado")
	}
	var doc struct {
		Props struct {
			PageProps json.RawMessage `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(m[1], &doc); err != nil {
		return fmt.Errorf("__NEXT_DATA__: %w", err)
	}
	return json.Unmarshal(doc.Props.PageProps, out)
}

// webHost is the website's host name, without port (as the jar stores it).
func (c *Client) webHost() string {
	u, _ := url.Parse(c.URLs.Web)
	return u.Hostname()
}

// htmlPage is a fetched HTML page after following redirects.
type htmlPage struct {
	url     *url.URL
	status  int
	body    []byte
	forms   []htmlForm
	message string // Keycloak feedback text
}

func (c *Client) fetchPage(ctx context.Context, method, target string, form url.Values) (*htmlPage, error) {
	r := request{method: method, url: target, accept: "text/html,application/xhtml+xml"}
	if form != nil {
		r.body = form
	}
	resp, err := c.send(ctx, r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	p := &htmlPage{url: resp.Request.URL, status: resp.StatusCode, body: body}
	if doc, err := html.Parse(bytes.NewReader(body)); err == nil {
		p.forms, p.message = parseHTML(doc, resp.Request.URL)
	}
	return p, nil
}

type htmlInput struct{ Name, Type, Value string }

type htmlForm struct {
	ID, Action string
	Inputs     []htmlInput
}

func (f htmlForm) has(name string) bool {
	for _, in := range f.Inputs {
		if in.Name == name {
			return true
		}
	}
	return false
}

// values returns the inputs that carry a value (hidden fields and the like).
func (f htmlForm) values() url.Values {
	v := url.Values{}
	for _, in := range f.Inputs {
		if in.Name != "" && in.Type != "password" && in.Type != "submit" && in.Value != "" {
			v.Set(in.Name, in.Value)
		}
	}
	return v
}

func findForm(forms []htmlForm, match func(htmlForm) bool) (htmlForm, bool) {
	for _, f := range forms {
		if match(f) {
			return f, true
		}
	}
	return htmlForm{}, false
}

func parseHTML(doc *html.Node, base *url.URL) ([]htmlForm, string) {
	var forms []htmlForm
	var message string
	var current *htmlForm
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "form":
				f := htmlForm{ID: attr(n, "id"), Action: resolve(base, attr(n, "action"))}
				forms = append(forms, f)
				current = &forms[len(forms)-1]
				for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
					walk(ch)
				}
				current = nil
				return
			case "input":
				if current != nil {
					current.Inputs = append(current.Inputs, htmlInput{attr(n, "name"), strings.ToLower(attr(n, "type")), attr(n, "value")})
				}
			case "span", "div", "p":
				id, class := attr(n, "id"), attr(n, "class")
				if message == "" && (strings.HasPrefix(id, "input-error") || strings.Contains(class, "kc-feedback-text") || strings.Contains(class, "alert-error")) {
					message = strings.Join(strings.Fields(text(n)), " ")
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return forms, message
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func text(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		b.WriteString(text(ch))
		b.WriteString(" ")
	}
	return b.String()
}

func resolve(base *url.URL, ref string) string {
	u, err := url.Parse(ref)
	if err != nil || base == nil {
		return ref
	}
	return base.ResolveReference(u).String()
}
