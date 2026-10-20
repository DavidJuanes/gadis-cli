// Package gadis is an HTTP client for gadisline.com, the online shop of the
// Gadis supermarket chain. The API is private and undocumented; see
// docs/api.md for how each call was worked out.
package gadis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// SiteID identifies Gadisline among the sites served by the same backend.
	SiteID = "56df88f9-479f-4361-891e-e1864dba1ca3"
	// DefaultStoreID is the assortment the website shows before a postal code is chosen.
	DefaultStoreID = "891d5c1e-a7a0-4287-9ea3-30c5703a4f63"

	sessionCookie = "__Secure-next-auth.session-token"
	userAgent     = "gadis-cli (+https://github.com/DavidJuanes/gadis-cli)"
)

// Endpoints holds the base URLs of the services. Tests point them at httptest servers.
type Endpoints struct {
	Web     string // https://www.gadisline.com
	Catalog string // https://catalog.gadisline.com/api/v3
	Store   string // https://store.gadisline.com/api/v3
	Cart    string // https://cart.gadisline.com/api/v3
	Clients string // https://clients.gadisline.com/api/v3
}

// Production are the real gadisline.com services.
var Production = Endpoints{
	Web:     "https://www.gadisline.com",
	Catalog: "https://catalog.gadisline.com/api/v3",
	Store:   "https://store.gadisline.com/api/v3",
	Cart:    "https://cart.gadisline.com/api/v3",
	Clients: "https://clients.gadisline.com/api/v3",
}

// Client talks to gadisline.com. It is not safe for concurrent use.
type Client struct {
	HTTP *http.Client
	Jar  *Jar
	URLs Endpoints
}

// New returns a client that stores its session in jar.
func New(jar *Jar) *Client {
	return &Client{
		HTTP: &http.Client{Jar: jar, Timeout: 30 * time.Second},
		Jar:  jar,
		URLs: Production,
	}
}

// APIError is a non-2xx response.
type APIError struct {
	Method, URL string
	Status      int
	Body        string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 300 {
		body = body[:300] + "…"
	}
	return fmt.Sprintf("%s %s: HTTP %d %s", e.Method, e.URL, e.Status, body)
}

type request struct {
	method  string
	url     string
	query   url.Values
	body    any // JSON-encoded unless it is url.Values (form)
	storeID string
	token   string
	accept  string // default application/json
	out     any    // decoded from JSON when non-nil
}

func (c *Client) do(ctx context.Context, r request) error {
	resp, err := c.send(ctx, r)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{Method: r.method, URL: r.url, Status: resp.StatusCode, Body: string(data)}
	}
	if r.out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, r.out); err != nil {
		return fmt.Errorf("%s %s: unexpected response: %w", r.method, r.url, err)
	}
	return nil
}

// send performs the request and saves the cookie jar, whatever the outcome.
func (c *Client) send(ctx context.Context, r request) (*http.Response, error) {
	u := r.url
	if len(r.query) > 0 {
		u += "?" + r.query.Encode()
	}
	var body io.Reader
	contentType := ""
	switch b := r.body.(type) {
	case nil:
	case url.Values:
		body = strings.NewReader(b.Encode())
		contentType = "application/x-www-form-urlencoded"
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
		contentType = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, r.method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	accept := r.accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "es")
	req.Header.Set("Time-Zone", "Europe/Madrid")
	req.Header.Set("site-id", SiteID)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if r.storeID != "" {
		req.Header.Set("store-id", r.storeID)
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	resp, err := c.HTTP.Do(req)
	if saveErr := c.Jar.Save(); saveErr != nil && err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("saving session: %w", saveErr)
	}
	return resp, err
}
