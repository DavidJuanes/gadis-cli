package gadis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StoredCookie is the on-disk form of a cookie. Unlike net/http/cookiejar,
// it keeps domain, path and expiry so the session survives between runs.
type StoredCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`    // without leading dot
	HostOnly bool      `json:"host_only"` // true if the server sent no Domain attribute
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitzero"` // zero = session cookie
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"http_only"`
}

func (c StoredCookie) expired(now time.Time) bool {
	return !c.Expires.IsZero() && !c.Expires.After(now)
}

// Jar is a small persistent http.CookieJar. NextAuth rotates its session
// cookies when it refreshes the access token, so callers must Save after
// every request; Client does that for them.
type Jar struct {
	mu      sync.Mutex
	path    string
	cookies []StoredCookie
	now     func() time.Time
}

// LoadJar reads the jar at path. A missing file yields an empty jar.
func LoadJar(path string) (*Jar, error) {
	j := &Jar{path: path, now: time.Now}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &j.cookies); err != nil {
		return nil, err
	}
	return j, nil
}

// Save writes the jar to disk with 0600 permissions, dropping expired cookies.
func (j *Jar) Save() error {
	if j.path == "" {
		return nil
	}
	j.mu.Lock()
	j.purge()
	data, err := json.MarshalIndent(j.cookies, "", "  ")
	j.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

// Clear removes every cookie (in memory; call Save to persist).
func (j *Jar) Clear() {
	j.mu.Lock()
	j.cookies = nil
	j.mu.Unlock()
}

// Has reports whether a live cookie with that name exists for the host.
func (j *Jar) Has(host, name string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.clock()
	for _, c := range j.cookies {
		if c.Name == name && !c.expired(now) && domainMatch(c, host) {
			return true
		}
	}
	return false
}

// Put adds or replaces a cookie, as if a server had set it.
func (j *Jar) Put(c StoredCookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	c.Domain = strings.TrimPrefix(strings.ToLower(c.Domain), ".")
	if c.Path == "" {
		c.Path = "/"
	}
	for i, old := range j.cookies {
		if old.Name == c.Name && old.Domain == c.Domain && old.Path == c.Path {
			j.cookies[i] = c
			return
		}
	}
	j.cookies = append(j.cookies, c)
}

// SetCookies implements http.CookieJar.
func (j *Jar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	now := j.clock()
	host := strings.ToLower(u.Hostname())
	for _, hc := range cookies {
		c := StoredCookie{
			Name:     hc.Name,
			Value:    hc.Value,
			Path:     hc.Path,
			Secure:   hc.Secure,
			HTTPOnly: hc.HttpOnly,
		}
		if c.Path == "" || !strings.HasPrefix(c.Path, "/") {
			c.Path = defaultPath(u.Path)
		}
		if hc.Domain == "" {
			c.Domain, c.HostOnly = host, true
		} else {
			d := strings.TrimPrefix(strings.ToLower(hc.Domain), ".")
			if host != d && !strings.HasSuffix(host, "."+d) {
				continue // a server may not set cookies for another domain
			}
			c.Domain = d
		}
		switch {
		case hc.MaxAge < 0:
			c.Expires = now.Add(-time.Second) // delete
		case hc.MaxAge > 0:
			c.Expires = now.Add(time.Duration(hc.MaxAge) * time.Second)
		case !hc.Expires.IsZero():
			c.Expires = hc.Expires
		}
		j.Put(c)
	}
	j.mu.Lock()
	j.purge()
	j.mu.Unlock()
}

// Cookies implements http.CookieJar.
func (j *Jar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.clock()
	host := strings.ToLower(u.Hostname())
	path := u.Path
	if path == "" {
		path = "/"
	}
	var out []*http.Cookie
	for _, c := range j.cookies {
		if c.expired(now) || !domainMatch(c, host) || !pathMatch(c.Path, path) {
			continue
		}
		if c.Secure && u.Scheme != "https" {
			continue
		}
		out = append(out, &http.Cookie{Name: c.Name, Value: c.Value})
	}
	return out
}

func (j *Jar) clock() time.Time {
	if j.now == nil {
		return time.Now()
	}
	return j.now()
}

func (j *Jar) purge() {
	now := j.clock()
	live := j.cookies[:0]
	for _, c := range j.cookies {
		if !c.expired(now) {
			live = append(live, c)
		}
	}
	j.cookies = live
}

func domainMatch(c StoredCookie, host string) bool {
	if c.HostOnly {
		return host == c.Domain
	}
	return host == c.Domain || strings.HasSuffix(host, "."+c.Domain)
}

func pathMatch(cookiePath, reqPath string) bool {
	if cookiePath == reqPath || cookiePath == "/" {
		return true
	}
	if !strings.HasPrefix(reqPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") || reqPath[len(cookiePath)] == '/'
}

func defaultPath(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}
