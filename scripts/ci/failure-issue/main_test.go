package main

import (
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/scripts/ci/internal/fakegh"
)

// The fake gh answers `gh issue list` with the fixture of open issues.
func TestMain(m *testing.M) {
	fakegh.Main(m, func(args []string) bool {
		return len(args) >= 2 && args[0] == "issue" && args[1] == "list"
	})
}

func containsPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
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

func TestFailureWithoutOpenIssueOpensNeedsTriageIssue(t *testing.T) {
	fake := fakegh.Install(t, "no-issues.json")

	if err := run([]string{"--status", "failure"}, runEnv("vuln-scheduled")); err != nil {
		t.Fatal(err)
	}

	w := fake.Writes()
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
	fake := fakegh.Install(t, "open-issues.json")

	if err := run([]string{"--status", "failure"}, runEnv("vuln-scheduled")); err != nil {
		t.Fatal(err)
	}

	w := fake.Writes()
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
	fake := fakegh.Install(t, "open-issues.json")

	if err := run([]string{"--status", "success"}, runEnv("vuln-scheduled")); err != nil {
		t.Fatal(err)
	}

	w := fake.Writes()
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
			fake := fakegh.Install(t, tc.fixture)
			if err := run([]string{"--status", tc.status}, runEnv("vuln-scheduled")); err != nil {
				t.Fatal(err)
			}
			if w := fake.Writes(); len(w) != 0 {
				t.Fatalf("want no write calls, got %+v", w)
			}
		})
	}
}

func TestUnknownStatusIsAnError(t *testing.T) {
	fakegh.Install(t, "no-issues.json")
	if err := run([]string{"--status", "flaky"}, runEnv("vuln-scheduled")); err == nil {
		t.Fatal("want error for unknown status")
	}
}

func TestGhFailureIsReported(t *testing.T) {
	fakegh.Install(t, "no-issues.json").Fail()
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
