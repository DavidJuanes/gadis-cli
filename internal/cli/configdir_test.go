package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// Without a known settings directory (e.g. no $HOME), only the commands that
// need it fail: --version and help still work.
func TestUnknownConfigDirFailsOnlyCommandsThatNeedIt(t *testing.T) {
	fake := gadistest.New(t)
	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := cli.Run(context.Background(), cli.Env{
			Args: args, Stdout: &stdout, Stderr: &stderr,
			URLs: fake.Endpoints(), Transport: fake.Transport(),
			ResolveConfigDir: func() (string, error) { return "", errors.New("$HOME is not defined") },
		})
		return code, stdout.String(), stderr.String()
	}

	for _, args := range [][]string{{"--version"}, {"--help"}, {"cart", "--help"}} {
		if code, stdout, stderr := run(args...); code != 0 || stdout == "" {
			t.Errorf("%v: code %d, stdout %q, stderr %q; want success", args, code, stdout, stderr)
		}
	}

	code, _, stderr := run("search", "leche")
	if code == 0 || !strings.Contains(stderr, "$HOME is not defined") {
		t.Errorf("search: code %d, stderr %q; want a failure naming the cause", code, stderr)
	}
}
