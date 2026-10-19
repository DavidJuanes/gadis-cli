// Package config handles the settings directory (~/.gadis): the user
// settings and the session file.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is <dir>/config.toml. It holds no secrets.
type Config struct {
	// PostalCode overrides the account's default postal code for catalog
	// commands. Empty = use the account's (or the website's default store).
	PostalCode string `toml:"postal_code,omitempty"`
}

// Dir is the settings directory: $GADIS_HOME, or ~/.gadis.
func Dir() (string, error) {
	if d := os.Getenv("GADIS_HOME"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gadis"), nil
}

// SessionPath is where the session cookies are kept (0600).
func SessionPath(dir string) string { return filepath.Join(dir, "session.json") }

// BrowserProfile is the Chromium profile used by `gadis login --browser`.
func BrowserProfile(dir string) string { return filepath.Join(dir, "chromium") }

func path(dir string) string { return filepath.Join(dir, "config.toml") }

// Load reads config.toml from dir; a missing file is an empty config.
func Load(dir string) (Config, error) {
	var c Config
	_, err := toml.DecodeFile(path(dir), &c)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	return c, err
}

// Save writes config.toml in dir.
func Save(dir string, c Config) error {
	p := path(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
