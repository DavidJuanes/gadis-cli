package cli_test

import (
	"bytes"
	"context"
	"runtime/debug"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli"
)

// versionOutput runs --version with read as the build-info reader and returns
// what it prints.
func versionOutput(t *testing.T, read func() (*debug.BuildInfo, bool)) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Args: []string{"--version"}, Stdout: &stdout, Stderr: &stderr, ReadBuildInfo: read,
	})
	if code != 0 {
		t.Fatalf("--version: code %d, stderr %q", code, stderr.String())
	}
	return stdout.String()
}

func buildInfo(version string) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: version}}, true
	}
}

// A binary built with `go install …@vX.Y.Z` has no -ldflags version, but Go
// records the module version in the build info: --version shows it, without
// the v, like the GoReleaser builds.
func TestVersionFallsBackToBuildInfo(t *testing.T) {
	for build, want := range map[string]string{
		"v0.3.1":  "gadis version 0.3.1\n",
		"(devel)": "gadis version dev\n",
		"":        "gadis version dev\n",
	} {
		if got := versionOutput(t, buildInfo(build)); got != want {
			t.Errorf("build info %q: --version = %q, want %q", build, got, want)
		}
	}
}

// Without build info (or without a reader), --version stays "dev".
func TestVersionWithoutBuildInfo(t *testing.T) {
	for name, read := range map[string]func() (*debug.BuildInfo, bool){
		"no reader":     nil,
		"no build info": func() (*debug.BuildInfo, bool) { return nil, false },
	} {
		if got := versionOutput(t, read); got != "gadis version dev\n" {
			t.Errorf("%s: --version = %q, want %q", name, got, "gadis version dev\n")
		}
	}
}

// The -ldflags version (GoReleaser) wins over the build info.
func TestLdflagsVersionWinsOverBuildInfo(t *testing.T) {
	saved := cli.Version
	t.Cleanup(func() { cli.Version = saved })
	cli.Version = "0.4.0"
	if got := versionOutput(t, buildInfo("v0.3.1")); got != "gadis version 0.4.0\n" {
		t.Errorf("--version = %q, want %q", got, "gadis version 0.4.0\n")
	}
}
