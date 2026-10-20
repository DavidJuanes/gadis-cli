//go:build integration

package clitest

import (
	"os"
	"path/filepath"
	"testing"
)

// sessionDirs are prepared session directories that hold no valid session.
func sessionDirs(t *testing.T) map[string]string {
	expired := t.TempDir()
	if err := os.WriteFile(filepath.Join(expired, "session.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"unset":   "",
		"no file": t.TempDir(),
		"expired": expired, // no cookies: whoami says there is no session
	}
}

// Without GADIS_INTEGRATION_ACCOUNT=1 the account tests are skipped, never
// failed, whatever the prepared session.
func TestProductionWithSessionSkipsUnlessAccountTestsAreOn(t *testing.T) {
	t.Setenv(AccountTestsEnv, "")
	for name, dir := range sessionDirs(t) {
		t.Setenv(SessionDirEnv, dir)
		r := record(t, func(t testing.TB) {
			ProductionWithSession(t)
			t.Error("did not skip")
		})
		if !r.skipped || r.failed {
			t.Errorf("%s: skipped %v, failed %v; want skipped: %s", name, r.skipped, r.failed, r.output())
		}
	}
}

// With GADIS_INTEGRATION_ACCOUNT=1 (CI after `gadis login`) a missing or
// invalid session fails the tests, so a broken login can't pass as skipped
// tests.
func TestProductionWithSessionFailsWithoutSessionWhenAccountTestsAreOn(t *testing.T) {
	t.Setenv(AccountTestsEnv, "1")
	for name, dir := range sessionDirs(t) {
		t.Setenv(SessionDirEnv, dir)
		r := record(t, func(t testing.TB) {
			ProductionWithSession(t)
			t.Error("returned a CLI")
		})
		if r.skipped || !r.failed {
			t.Errorf("%s: skipped %v, failed %v; want failed: %s", name, r.skipped, r.failed, r.output())
		}
	}
}
