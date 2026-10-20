package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test binary doubles as a fake `gh`: when invoked through a symlink
// named "gh" it records the call and answers `issue list` from a fixture.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "gh" {
		os.Exit(fakeGh())
	}
	os.Exit(m.Run())
}

type ghCall struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

func fakeGh() int {
	args := os.Args[1:]
	var stdin []byte
	if containsPair(args, "--body-file", "-") {
		stdin, _ = io.ReadAll(os.Stdin)
	}
	line, _ := json.Marshal(ghCall{Args: args, Stdin: string(stdin)})
	f, err := os.OpenFile(os.Getenv("FAKE_GH_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 2
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return 2
	}
	if os.Getenv("FAKE_GH_FAIL") != "" {
		return 1
	}
	if len(args) >= 2 && args[0] == "issue" && args[1] == "list" {
		data, err := os.ReadFile(os.Getenv("FAKE_GH_ISSUES"))
		if err != nil {
			return 2
		}
		_, _ = os.Stdout.Write(data)
	}
	return 0
}

func containsPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}

// fakeGhEnv puts the fake gh first on PATH, serving the given fixture of open
// issues, and returns a function that reads the recorded calls.
func fakeGhEnv(t *testing.T, issuesFixture string) func() []ghCall {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "gh")); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join("testdata", issuesFixture))
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.jsonl")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_LOG", log)
	t.Setenv("FAKE_GH_ISSUES", abs)
	return func() []ghCall {
		f, err := os.Open(log)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var calls []ghCall
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var c ghCall
			if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
				t.Fatal(err)
			}
			calls = append(calls, c)
		}
		return calls
	}
}

func runEnv(workflow string) func(string) string {
	env := map[string]string{
		"GITHUB_WORKFLOW":   workflow,
		"GITHUB_SERVER_URL": "https://github.com",
		"GITHUB_REPOSITORY": "DavidJuanes/gadis-cli",
		"GITHUB_RUN_ID":     "123456",
	}
	return func(k string) string { return env[k] }
}

const runURL = "https://github.com/DavidJuanes/gadis-cli/actions/runs/123456"

// writes drops the read-only lookups and keeps the calls that change issues.
func writes(calls []ghCall) []ghCall {
	var out []ghCall
	for _, c := range calls {
		if len(c.Args) >= 2 && c.Args[0] == "issue" && c.Args[1] == "list" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func TestFailureWithoutOpenIssueOpensNeedsTriageIssue(t *testing.T) {
	calls := fakeGhEnv(t, "no-issues.json")

	if err := run([]string{"--status", "failure"}, runEnv("vuln-scheduled")); err != nil {
		t.Fatal(err)
	}

	w := writes(calls())
	if len(w) != 1 {
		t.Fatalf("want 1 write call, got %+v", w)
	}
	c := w[0]
	if c.Args[0] != "issue" || c.Args[1] != "create" {
		t.Fatalf("want issue create, got %v", c.Args)
	}
	if !containsPair(c.Args, "--label", "needs-triage") {
		t.Errorf("missing needs-triage label: %v", c.Args)
	}
	if !containsPair(c.Args, "--title", "Falla el workflow programado vuln-scheduled") {
		t.Errorf("unexpected title: %v", c.Args)
	}
	if !strings.Contains(c.Stdin, runURL) {
		t.Errorf("body lacks run link: %q", c.Stdin)
	}
	if !strings.Contains(c.Stdin, "<!-- failure-issue: vuln-scheduled -->") {
		t.Errorf("body lacks marker: %q", c.Stdin)
	}
}

func TestFailureWithOpenIssueCommentsOnIt(t *testing.T) {
	calls := fakeGhEnv(t, "open-issues.json")

	if err := run([]string{"--status", "failure"}, runEnv("vuln-scheduled")); err != nil {
		t.Fatal(err)
	}

	w := writes(calls())
	if len(w) != 1 {
		t.Fatalf("want 1 write call, got %+v", w)
	}
	c := w[0]
	if len(c.Args) < 3 || c.Args[0] != "issue" || c.Args[1] != "comment" || c.Args[2] != "42" {
		t.Fatalf("want issue comment 42, got %v", c.Args)
	}
	if !strings.Contains(c.Stdin, runURL) {
		t.Errorf("comment lacks run link: %q", c.Stdin)
	}
}

func TestSuccessClosesOpenIssueWithComment(t *testing.T) {
	calls := fakeGhEnv(t, "open-issues.json")

	if err := run([]string{"--status", "success"}, runEnv("vuln-scheduled")); err != nil {
		t.Fatal(err)
	}

	w := writes(calls())
	if len(w) != 1 {
		t.Fatalf("want 1 write call, got %+v", w)
	}
	c := w[0]
	if len(c.Args) < 3 || c.Args[0] != "issue" || c.Args[1] != "close" || c.Args[2] != "42" {
		t.Fatalf("want issue close 42, got %v", c.Args)
	}
	i := indexOf(c.Args, "--comment")
	if i < 0 || i+1 >= len(c.Args) || !strings.Contains(c.Args[i+1], runURL) {
		t.Errorf("close lacks a comment with the run link: %v", c.Args)
	}
}

func TestNothingToDo(t *testing.T) {
	cases := []struct {
		name, fixture, status string
	}{
		{"success without open issue", "no-issues.json", "success"},
		{"cancelled run leaves the issue alone", "open-issues.json", "cancelled"},
		{"skipped run leaves the issue alone", "open-issues.json", "skipped"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := fakeGhEnv(t, tc.fixture)
			if err := run([]string{"--status", tc.status}, runEnv("vuln-scheduled")); err != nil {
				t.Fatal(err)
			}
			if w := writes(calls()); len(w) != 0 {
				t.Fatalf("want no write calls, got %+v", w)
			}
		})
	}
}

func TestUnknownStatusIsAnError(t *testing.T) {
	fakeGhEnv(t, "no-issues.json")
	if err := run([]string{"--status", "flaky"}, runEnv("vuln-scheduled")); err == nil {
		t.Fatal("want error for unknown status")
	}
}

func TestGhFailureIsReported(t *testing.T) {
	fakeGhEnv(t, "no-issues.json")
	t.Setenv("FAKE_GH_FAIL", "1")
	if err := run([]string{"--status", "failure"}, runEnv("vuln-scheduled")); err == nil {
		t.Fatal("want error when gh fails")
	}
}

func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}
