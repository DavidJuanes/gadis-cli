// Package gadistest is a fake Gadisline backend for tests: an httptest TLS
// server (NextAuth cookies are Secure) that mimics the private API documented
// in docs/api.md, including its quirks.
//
// The fake is split by area, one file each, so they can grow independently:
//
//   - session.go: NextAuth + Keycloak login and the home page with __NEXT_DATA__
//   - catalog.go: product search (catalog service)
//   - stores.go:  postal codes (store service)
//   - cart.go:    the account's cart
//
// Each area is a struct with its state and a register method that adds its
// routes to the mux. Every request is recorded so tests can assert on what the
// backend received.
package gadistest

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

// SiteID is the site-id header every microservice call must carry. It is
// spelled out here, not taken from package gadis, so the fake checks the
// client against an independent value.
const SiteID = "56df88f9-479f-4361-891e-e1864dba1ca3"

// Base paths of each service on the fake server. They differ so a call sent
// to the wrong service gets a 404.
const (
	catalogPrefix = "/catalog-api"
	storePrefix   = "/store-api"
	cartPrefix    = "/cart-api"
	clientsPrefix = "/clients-api"
)

// Server is the fake Gadisline. Configure its areas before running commands.
type Server struct {
	Session *Session
	Catalog *Catalog
	Stores  *Stores
	Cart    *Cart

	srv *httptest.Server

	mu       sync.Mutex
	requests []Request
	failures map[string]int // "METHOD /path" → status
}

// Request is a request received by the fake.
type Request struct {
	Method string
	Path   string // without the service prefix of the fake, e.g. /catalog/products/search
	Query  string
	Header http.Header
	Body   []byte
}

// New starts a fake Gadisline with default data, closed when the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		Session: newSession(),
		Catalog: newCatalog(),
		Stores:  newStores(),
		Cart:    newCart(),
	}
	mux := http.NewServeMux()
	s.Session.register(mux, s)
	s.Catalog.register(mux, s)
	s.Stores.register(mux, s)
	s.Cart.register(mux, s)
	s.srv = httptest.NewTLSServer(s.record(mux))
	t.Cleanup(s.srv.Close)
	return s
}

// URL is the base URL of the fake (the website).
func (s *Server) URL() string { return s.srv.URL }

// Endpoints are the service URLs to give the client or the CLI.
func (s *Server) Endpoints() gadis.Endpoints {
	return gadis.Endpoints{
		Web:     s.srv.URL,
		Catalog: s.srv.URL + catalogPrefix,
		Store:   s.srv.URL + storePrefix,
		Cart:    s.srv.URL + cartPrefix,
		Clients: s.srv.URL + clientsPrefix,
	}
}

// Transport trusts the fake's TLS certificate.
func (s *Server) Transport() http.RoundTripper { return s.srv.Client().Transport }

// Client returns a gadis client wired to the fake, with an in-memory jar.
func (s *Server) Client() *gadis.Client {
	c := gadis.New(&gadis.Jar{})
	c.HTTP.Transport = s.Transport()
	c.URLs = s.Endpoints()
	return c
}

// Requests returns the requests received so far whose method and path match
// (an empty method matches any). Paths are given without the service prefix.
func (s *Server) Requests(method, path string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Request
	for _, r := range s.requests {
		if (method == "" || r.Method == method) && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// Fail makes every later request to method and path (without the service
// prefix) answer with status, whatever the area would do. Status 0 undoes it.
func (s *Server) Fail(method, path string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures == nil {
		s.failures = map[string]int{}
	}
	s.failures[method+" "+path] = status
}

// AllRequests returns every request received so far.
func (s *Server) AllRequests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *Server) record(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		path := r.URL.Path
		for _, p := range []string{catalogPrefix, storePrefix, cartPrefix, clientsPrefix} {
			if strings.HasPrefix(path, p+"/") {
				path = strings.TrimPrefix(path, p)
				break
			}
		}
		s.mu.Lock()
		s.requests = append(s.requests, Request{
			Method: r.Method, Path: path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body,
		})
		status := s.failures[r.Method+" "+path]
		s.mu.Unlock()
		if status != 0 {
			http.Error(w, `{"message":"fake failure"}`, status)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// lock guards the areas' state against concurrent handlers.
func (s *Server) lock() func() {
	s.mu.Lock()
	return s.mu.Unlock
}

// requireSite rejects microservice calls without the right site-id header,
// as the real gateway does.
func requireSite(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("site-id") != SiteID {
		http.Error(w, `{"message":"site-id required"}`, http.StatusBadRequest)
		return false
	}
	return true
}
