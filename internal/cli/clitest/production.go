//go:build integration

package clitest

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/config"
	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

const (
	// AccountTestsEnv names the environment variable that turns on the
	// account and cart tests: they run only when it is "1".
	AccountTestsEnv = "GADIS_INTEGRATION_ACCOUNT"
	// SessionDirEnv names the environment variable with the settings
	// directory (like GADIS_HOME) that holds the session prepared for the
	// account and cart tests.
	SessionDirEnv = "GADIS_TEST_HOME"
)

const sessionHelp = "prepare a session with `GADIS_HOME=<dir> gadis login` and run the tests with " +
	AccountTestsEnv + "=1 " + SessionDirEnv + "=<dir> go test -tags integration ./..."

// Production returns a CLI wired to the real gadisline.com, with a fresh
// settings directory: no session and no saved postal code.
func Production(t testing.TB) *CLI {
	t.Helper()
	return &CLI{URLs: gadis.Production, Dir: t.TempDir(), Env: map[string]string{}}
}

// ProductionWithSession returns a private CLI (see CLI.Private) wired to the
// real gadisline.com with the session prepared in $GADIS_TEST_HOME. The
// session file is copied into a fresh settings directory, so tests never
// change the prepared directory (rotated cookies, saved postal code) and the
// catalog uses the account's postal code.
//
// It skips the test unless $GADIS_INTEGRATION_ACCOUNT is "1". When it is, a
// missing or invalid session (checked with `gadis whoami`) fails the test:
// the account tests were asked for and can't run.
func ProductionWithSession(t testing.TB) *CLI {
	t.Helper()
	if os.Getenv(AccountTestsEnv) != "1" {
		t.Skipf("account tests are off (%s is not 1): %s", AccountTestsEnv, sessionHelp)
	}
	src := os.Getenv(SessionDirEnv)
	if src == "" {
		t.Fatalf("no prepared session (%s is empty): %s", SessionDirEnv, sessionHelp)
	}
	session, err := os.ReadFile(config.SessionPath(src))
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("no session in %s: %s", src, sessionHelp)
	}
	if err != nil {
		t.Fatalf("reading the prepared session: %v", err)
	}
	c := Production(t)
	c.Private = true
	if err := os.WriteFile(config.SessionPath(c.Dir), session, 0o600); err != nil {
		t.Fatal(err)
	}
	if res := c.Run(t, "whoami"); res.Code != 0 {
		if strings.Contains(res.Stderr, gadis.ErrNotLoggedIn.Error()) {
			t.Fatalf("the session in %s is invalid or expired: %s", src, sessionHelp)
		}
		t.Fatalf("whoami with the prepared session: exit code %d (%s)", res.Code, hidden)
	}
	return c
}
