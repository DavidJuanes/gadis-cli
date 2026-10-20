// Package fakegh is a fake GitHub CLI for the tests of scripts/ci. The test
// binary doubles as `gh`: Install puts a symlink named gh to it first on
// PATH, and Main, called from TestMain, makes it record each call and answer
// the calls that read from a fixture.
package fakegh

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Call is one invocation of the fake gh.
type Call struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

// isRead tells the calls that read (answered with the fixture, left out by
// Writes) from those that write. Main sets it.
var isRead func(args []string) bool

// Main runs the tests, or acts as gh when the binary is invoked as gh. read
// tells the calls that read from GitHub, which the fake answers with the
// fixture given to Install.
func Main(m *testing.M, read func(args []string) bool) {
	isRead = read
	if filepath.Base(os.Args[0]) == "gh" {
		os.Exit(serve())
	}
	os.Exit(m.Run())
}

const (
	envLog     = "FAKE_GH_LOG"
	envFixture = "FAKE_GH_FIXTURE"
	envFail    = "FAKE_GH_FAIL"
)

func serve() int {
	args := os.Args[1:]
	stdin, _ := io.ReadAll(os.Stdin)
	line, _ := json.Marshal(Call{Args: args, Stdin: string(stdin)})
	f, err := os.OpenFile(os.Getenv(envLog), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 2
	}
	_, err = f.Write(append(line, '\n'))
	if closeErr := f.Close(); err != nil || closeErr != nil {
		return 2
	}
	if os.Getenv(envFail) != "" {
		return 1
	}
	if isRead(args) {
		data, err := os.ReadFile(os.Getenv(envFixture))
		if err != nil {
			return 2
		}
		_, _ = os.Stdout.Write(data)
	}
	return 0
}

// Fake is a fake gh installed for one test.
type Fake struct {
	t   *testing.T
	log string
}

// Install puts the fake gh first on PATH for the test, answering the calls
// that read with testdata/fixture.
func Install(t *testing.T, fixture string) *Fake {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "gh")); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.jsonl")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(envLog, log)
	t.Setenv(envFixture, abs)
	return &Fake{t: t, log: log}
}

// Fail makes every later call record itself and exit with status 1.
func (f *Fake) Fail() { f.t.Setenv(envFail, "1") }

// Calls are the calls recorded so far, in order.
func (f *Fake) Calls() []Call {
	f.t.Helper()
	data, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var calls []Call
	for l := range strings.Lines(string(data)) {
		var c Call
		if err := json.Unmarshal([]byte(l), &c); err != nil {
			f.t.Fatal(err)
		}
		calls = append(calls, c)
	}
	return calls
}

// Writes are the recorded calls that write, the read-only lookups left out.
func (f *Fake) Writes() []Call {
	f.t.Helper()
	var out []Call
	for _, c := range f.Calls() {
		if !isRead(c.Args) {
			out = append(out, c)
		}
	}
	return out
}
