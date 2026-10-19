package gadis

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func names(cs []*http.Cookie) map[string]string {
	m := map[string]string{}
	for _, c := range cs {
		m[c.Name] = c.Value
	}
	return m
}

func TestJarDomainAndPathMatching(t *testing.T) {
	j := &Jar{now: time.Now}
	j.SetCookies(mustURL(t, "https://www.gadisline.com/api/auth/csrf"), []*http.Cookie{
		{Name: "hostonly", Value: "1", Path: "/"},
		{Name: "domain", Value: "2", Domain: ".gadisline.com", Path: "/"},
		{Name: "scoped", Value: "3", Path: "/api"},
		{Name: "secure", Value: "4", Path: "/", Secure: true},
		{Name: "foreign", Value: "5", Domain: "example.com"},
	})

	got := names(j.Cookies(mustURL(t, "https://www.gadisline.com/api/config/cart")))
	for _, n := range []string{"hostonly", "domain", "scoped", "secure"} {
		if _, ok := got[n]; !ok {
			t.Errorf("www /api: missing %s in %v", n, got)
		}
	}
	if _, ok := got["foreign"]; ok {
		t.Error("accepted a cookie for another domain")
	}

	got = names(j.Cookies(mustURL(t, "https://auth.gadisline.com/realms")))
	if len(got) != 1 || got["domain"] != "2" {
		t.Errorf("auth subdomain: got %v, want only the domain cookie", got)
	}

	got = names(j.Cookies(mustURL(t, "https://www.gadisline.com/apis")))
	if _, ok := got["scoped"]; ok {
		t.Error("path /api must not match /apis")
	}

	got = names(j.Cookies(mustURL(t, "http://www.gadisline.com/")))
	if _, ok := got["secure"]; ok {
		t.Error("secure cookie sent over http")
	}
}

func TestJarExpiryAndDeletion(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	j := &Jar{now: func() time.Time { return now }}
	u := mustURL(t, "https://www.gadisline.com/")
	j.SetCookies(u, []*http.Cookie{
		{Name: "short", Value: "a", MaxAge: 60},
		{Name: "dated", Value: "b", Expires: now.Add(time.Hour)},
		{Name: "gone", Value: "c", Expires: now.Add(-time.Hour)},
	})
	if got := names(j.Cookies(u)); len(got) != 2 || got["gone"] != "" {
		t.Fatalf("got %v, want short and dated", got)
	}

	j.SetCookies(u, []*http.Cookie{{Name: "short", Value: "", MaxAge: -1}})
	if j.Has("www.gadisline.com", "short") {
		t.Error("MaxAge<0 must delete the cookie")
	}

	now = now.Add(2 * time.Hour)
	if got := j.Cookies(u); len(got) != 0 {
		t.Errorf("after expiry got %v", names(got))
	}
}

func TestJarReplacesSameCookie(t *testing.T) {
	j := &Jar{now: time.Now}
	u := mustURL(t, "https://www.gadisline.com/")
	j.SetCookies(u, []*http.Cookie{{Name: "s", Value: "old", Path: "/"}})
	j.SetCookies(u, []*http.Cookie{{Name: "s", Value: "new", Path: "/"}})
	got := j.Cookies(u)
	if len(got) != 1 || got[0].Value != "new" {
		t.Errorf("got %v, want a single s=new", names(got))
	}
}

func TestJarSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "session.json")
	j, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	u := mustURL(t, "https://www.gadisline.com/")
	j.SetCookies(u, []*http.Cookie{
		{Name: sessionCookie + ".0", Value: "tok", Path: "/", Secure: true, HttpOnly: true, MaxAge: 3600},
	})
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file mode %o, want 600", perm)
	}

	loaded, err := LoadJar(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(loaded.Cookies(u)); got[sessionCookie+".0"] != "tok" {
		t.Errorf("after reload got %v", got)
	}
	if !New(loaded).LoggedIn() {
		t.Error("LoggedIn() = false with a chunked session cookie")
	}
}
