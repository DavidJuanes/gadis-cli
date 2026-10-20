package main

import (
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/scripts/ci/internal/ghtest"
)

// The fake gh answers `gh issue list` with the fixture of open issues.
func TestMain(m *testing.M) {
	ghtest.Main(m, func(args []string) bool {
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
	fake := ghtest.Install(t, "no-issues.json")

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
	fake := ghtest.Install(t, "open-issues.json")

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
	fake := ghtest.Install(t, "open-issues.json")

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
			fake := ghtest.Install(t, tc.fixture)
			if err := run([]string{"--status", tc.status}, runEnv("vuln-scheduled")); err != nil {
				t.Fatal(err)
			}
			if w := fake.Writes(); len(w) != 0 {
				t.Fatalf("want no write calls, got %+v", w)
			}
		})
	}
}

// integration.yml passes the results of its two jobs, catalog and account;
// only one of them runs, the other is skipped.
func TestSeveralResultsAreTheResultOfTheJobThatRan(t *testing.T) {
	cases := []struct {
		name, fixture, status, want string
	}{
		{"only active job failed", "no-issues.json", "skipped failure", "create"},
		{"only active job passed", "open-issues.json", "success skipped", "close"},
		{"only active job was cancelled", "open-issues.json", "skipped cancelled", ""},
		{"both skipped", "open-issues.json", "skipped skipped", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := ghtest.Install(t, tc.fixture)
			if err := run([]string{"--status", tc.status}, runEnv("vuln-scheduled")); err != nil {
				t.Fatal(err)
			}
			w := fake.Writes()
			if tc.want == "" {
				if len(w) != 0 {
					t.Fatalf("want no write calls, got %+v", w)
				}
				return
			}
			if len(w) != 1 || w[0].Args[1] != tc.want {
				t.Fatalf("want issue %s, got %+v", tc.want, w)
			}
		})
	}
}

func TestUnknownResultAmongSeveralIsAnError(t *testing.T) {
	ghtest.Install(t, "no-issues.json")
	if err := run([]string{"--status", "skipped flaky"}, runEnv("vuln-scheduled")); err == nil {
		t.Fatal("want error for unknown status")
	}
}

func TestUnknownStatusIsAnError(t *testing.T) {
	ghtest.Install(t, "no-issues.json")
	if err := run([]string{"--status", "flaky"}, runEnv("vuln-scheduled")); err == nil {
		t.Fatal("want error for unknown status")
	}
}

func TestGhFailureIsReported(t *testing.T) {
	ghtest.Install(t, "no-issues.json").Fail()
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
