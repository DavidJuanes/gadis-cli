//go:build integration

package clitest

import (
	"os"
	"path/filepath"
	"testing"
)

// Without a prepared session, session tests are skipped, never failed.
func TestProductionWithSessionSkipsWithoutSession(t *testing.T) {
	expired := t.TempDir()
	if err := os.WriteFile(filepath.Join(expired, "session.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{
		"unset":   "",
		"no file": t.TempDir(),
		"expired": expired, // no cookies: whoami says there is no session
	} {
		t.Setenv(SessionDirEnv, dir)
		var sub *testing.T
		t.Run(name, func(t *testing.T) {
			sub = t
			ProductionWithSession(t)
			t.Error("did not skip")
		})
		if !sub.Skipped() {
			t.Errorf("%s: not skipped", name)
		}
	}
}
