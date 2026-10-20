// Package gh runs the GitHub CLI for the scripts of scripts/ci.
package gh

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run runs gh with args and stdin on its standard input, and returns its
// standard output. The error carries gh's standard error.
func Run(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...) //nolint:gosec // G204: fixed binary, no shell; args are built by the caller
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
