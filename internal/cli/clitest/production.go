//go:build integration

package clitest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/config"
	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

// SessionDirEnv names the environment variable with the settings directory
// (like GADIS_HOME) that holds the session prepared for integration tests.
const SessionDirEnv = "GADIS_TEST_HOME"

const sessionHelp = "prepara una sesión con `GADIS_HOME=<dir> gadis login` y ejecuta los tests con " +
	SessionDirEnv + "=<dir> go test -tags integration ./..."

// Production returns a CLI wired to the real gadisline.com, with a fresh
// settings directory: no session and no saved postal code.
func Production(t testing.TB) *CLI {
	t.Helper()
	return &CLI{URLs: gadis.Production, Dir: t.TempDir(), Env: map[string]string{}}
}

// ProductionWithSession returns a CLI wired to the real gadisline.com with the
// session prepared in $GADIS_TEST_HOME. The session file is copied into a
// fresh settings directory, so tests never change the prepared directory
// (rotated cookies, saved postal code) and the catalog uses the account's
// postal code. It skips the test, with instructions, when there is no session
// or it is no longer valid (checked with `gadis whoami`).
func ProductionWithSession(t testing.TB) *CLI {
	t.Helper()
	src := os.Getenv(SessionDirEnv)
	if src == "" {
		t.Skipf("sin sesión preparada (%s vacía): %s", SessionDirEnv, sessionHelp)
	}
	session, err := os.ReadFile(config.SessionPath(src))
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("sin sesión en %s: %s", src, sessionHelp)
	}
	if err != nil {
		t.Fatalf("leyendo la sesión preparada: %v", err)
	}
	c := Production(t)
	if err := os.WriteFile(filepath.Join(c.Dir, filepath.Base(config.SessionPath(c.Dir))), session, 0o600); err != nil {
		t.Fatal(err)
	}
	if res := c.Run(t, "whoami"); res.Code != 0 {
		if strings.Contains(res.Stderr, gadis.ErrNotLoggedIn.Error()) {
			t.Skipf("la sesión de %s no es válida o ha caducado: %s", src, sessionHelp)
		}
		t.Fatalf("whoami con la sesión preparada: código %d\n%s", res.Code, res.Stderr)
	}
	return c
}
