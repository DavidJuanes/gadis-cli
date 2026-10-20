// Package clitest runs the gadis CLI in tests the way a user runs it: with
// arguments, stdin and a settings directory, against the fake Gadisline
// (unit tests) or the real one (integration tests), and captures stdout,
// stderr and the exit code.
package clitest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli"
	"github.com/DavidJuanes/gadis-cli/internal/gadis"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// CLI is a configured gadis command line. Its fields may be changed between
// runs (e.g. Stdin before a login).
type CLI struct {
	URLs      gadis.Endpoints
	Transport http.RoundTripper // nil = default transport
	Dir       string            // settings directory (GADIS_HOME)
	Stdin     string
	Terminal  bool              // whether stdin is a terminal
	Env       map[string]string // environment seen by the CLI; the process's is never read
	// Private marks the output as personal data (the maintainer's real
	// account and cart): a failing Result never prints it, only what went
	// wrong.
	Private bool

	fake *gadistest.Server
}

// Result is the outcome of a run.
type Result struct {
	Stdout, Stderr string
	Code           int
	Private        bool // copied from the CLI: the output is personal data
}

// New returns a CLI wired to the fake Gadisline, with a fresh settings
// directory and no terminal.
func New(t testing.TB, fake *gadistest.Server) *CLI {
	t.Helper()
	return &CLI{URLs: fake.Endpoints(), Transport: fake.Transport(), Dir: t.TempDir(), Env: map[string]string{}, fake: fake}
}

// Login logs in to the fake with its account's credentials (`gadis login`),
// so later runs have a session. Only for CLIs made with New.
func (c *CLI) Login(t testing.TB) {
	t.Helper()
	if c.fake == nil {
		t.Fatal("clitest: Login needs a CLI made with New (the fake Gadisline)")
	}
	s := c.fake.Session
	c.Run(t, "login", "--user", s.Email, "--password", s.Password).Success(t)
}

// Run executes `gadis args...` and returns what it printed and its exit code.
func (c *CLI) Run(t testing.TB, args ...string) Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Args:       args,
		Stdin:      strings.NewReader(c.Stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		URLs:       c.URLs,
		Transport:  c.Transport,
		ConfigDir:  c.Dir,
		IsTerminal: func() bool { return c.Terminal },
		Getenv:     func(k string) string { return c.Env[k] },
	})
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), Code: code, Private: c.Private}
}

// JSON decodes stdout into v, failing the test unless stdout is exactly one
// valid JSON document.
func (r Result) JSON(t testing.TB, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	if err := dec.Decode(v); err != nil {
		if r.Private {
			// The decode error quotes the offending value.
			t.Fatalf("stdout is not valid JSON for %T (%s)", v, hidden)
		}
		t.Fatalf("stdout is not valid JSON (%v):\n%s\nstderr:\n%s", err, r.Stdout, r.Stderr)
	}
	if dec.More() {
		if r.Private {
			t.Fatalf("stdout has more than one JSON document (%s)", hidden)
		}
		t.Fatalf("stdout has more than one JSON document:\n%s", r.Stdout)
	}
}

// Success fails the test unless the run exited with code 0.
func (r Result) Success(t testing.TB) Result {
	t.Helper()
	if r.Code != 0 && r.Private {
		t.Fatalf("exit code %d, want 0 (%s)", r.Code, hidden)
	}
	if r.Code != 0 {
		t.Fatalf("exit code %d, want 0\nstdout:\n%s\nstderr:\n%s", r.Code, r.Stdout, r.Stderr)
	}
	return r
}

// hidden stands for the output of a private run in failure messages.
const hidden = "output hidden: personal data"
