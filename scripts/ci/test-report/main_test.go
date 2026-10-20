package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const module = "example.com/demo"

func golden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// ciEnv is the environment of an Actions run whose step summary goes to a
// temporary file; it returns the getenv for run and the summary path.
func ciEnv(t *testing.T) (func(string) string, string) {
	t.Helper()
	summary := filepath.Join(t.TempDir(), "summary.md")
	env := map[string]string{
		"GITHUB_STEP_SUMMARY": summary,
		"GITHUB_REPOSITORY":   "DavidJuanes/gadis-cli",
	}
	return func(k string) string { return env[k] }, summary
}

func baseArgs() []string {
	return []string{
		"--junit", filepath.Join("testdata", "junit.xml"),
		"--coverprofile", filepath.Join("testdata", "cover.out"),
		"--module", module,
	}
}

func TestPushWritesSummaryAndAnnotatesFailures(t *testing.T) {
	getenv, summary := ciEnv(t)
	var stdout bytes.Buffer

	if err := run(baseArgs(), getenv, &stdout); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if want := golden(t, "summary.golden"); string(got) != want {
		t.Errorf("summary:\n%s\nwant:\n%s", got, want)
	}
	if want := golden(t, "annotations.txt"); stdout.String() != want {
		t.Errorf("annotations:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

// The test binary doubles as a fake `gh`: when invoked through a symlink
// named "gh" it records the call and answers the comment listing from a
// fixture.
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
	if slices.Contains(args, "body=@-") {
		stdin, _ = io.ReadAll(os.Stdin)
	}
	line, _ := json.Marshal(ghCall{Args: args, Stdin: string(stdin)})
	f, err := os.OpenFile(os.Getenv("FAKE_GH_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 2
	}
	_, err = f.Write(append(line, '\n'))
	if closeErr := f.Close(); err != nil || closeErr != nil {
		return 2
	}
	if os.Getenv("FAKE_GH_FAIL") != "" {
		return 1
	}
	if !slices.Contains(args, "--method") {
		data, err := os.ReadFile(os.Getenv("FAKE_GH_COMMENTS"))
		if err != nil {
			return 2
		}
		_, _ = os.Stdout.Write(data)
	}
	return 0
}

// fakeGhEnv puts the fake gh first on PATH, serving the given fixture of PR
// comments, and returns a function that reads the calls that write.
func fakeGhEnv(t *testing.T, commentsFixture string) func() []ghCall {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, "gh")); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join("testdata", commentsFixture))
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.jsonl")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_LOG", log)
	t.Setenv("FAKE_GH_COMMENTS", abs)
	return func() []ghCall {
		data, err := os.ReadFile(log)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		var calls []ghCall
		for l := range strings.Lines(string(data)) {
			var c ghCall
			if err := json.Unmarshal([]byte(l), &c); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(c.Args, "--method") {
				calls = append(calls, c)
			}
		}
		return calls
	}
}

// mainBadge serves the coverage badge of main, or 404 if body is empty, and
// returns its URL.
func mainBadge(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body == "" || r.URL.Path != "/badges/coverage.json" {
			http.Error(w, "404: Not Found", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/badges/coverage.json"
}

func prArgs(badgeURL string) []string {
	return append(baseArgs(), "--pr", "7", "--main-badge", badgeURL)
}

const main713 = `{"schemaVersion":1,"label":"cobertura","message":"71.3%","color":"yellow"}`

func TestPRWithoutCommentCreatesOneWithDiffAgainstMain(t *testing.T) {
	calls := fakeGhEnv(t, "no-comments.jsonl")
	getenv, _ := ciEnv(t)

	if err := run(prArgs(mainBadge(t, main713)), getenv, io.Discard); err != nil {
		t.Fatal(err)
	}

	w := calls()
	if len(w) != 1 {
		t.Fatalf("want 1 write call, got %+v", w)
	}
	want := []string{"api", "--method", "POST", "repos/DavidJuanes/gadis-cli/issues/7/comments", "-F", "body=@-"}
	if !slices.Equal(w[0].Args, want) {
		t.Errorf("args = %v, want %v", w[0].Args, want)
	}
	if want := golden(t, "comment-diff.golden"); w[0].Stdin != want {
		t.Errorf("comment:\n%s\nwant:\n%s", w[0].Stdin, want)
	}
}

// existing-comment.jsonl has the bot's comment (99) and a user's comment that
// quotes the marker (11), which must be left alone.
func TestPRWithCommentEditsTheBotsOne(t *testing.T) {
	calls := fakeGhEnv(t, "existing-comment.jsonl")
	getenv, _ := ciEnv(t)

	if err := run(prArgs(mainBadge(t, main713)), getenv, io.Discard); err != nil {
		t.Fatal(err)
	}

	w := calls()
	if len(w) != 1 {
		t.Fatalf("want 1 write call, got %+v", w)
	}
	want := []string{"api", "--method", "PATCH", "repos/DavidJuanes/gadis-cli/issues/comments/99", "-F", "body=@-"}
	if !slices.Equal(w[0].Args, want) {
		t.Errorf("args = %v, want %v", w[0].Args, want)
	}
	if want := golden(t, "comment-diff.golden"); w[0].Stdin != want {
		t.Errorf("comment:\n%s\nwant:\n%s", w[0].Stdin, want)
	}
}

func TestPRWithoutMainBadgeCommentsWithoutDiff(t *testing.T) {
	calls := fakeGhEnv(t, "no-comments.jsonl")
	getenv, _ := ciEnv(t)
	var stdout bytes.Buffer

	if err := run(prArgs(mainBadge(t, "")), getenv, &stdout); err != nil {
		t.Fatal(err)
	}

	w := calls()
	if len(w) != 1 {
		t.Fatalf("want 1 write call, got %+v", w)
	}
	if want := golden(t, "comment-no-main.golden"); w[0].Stdin != want {
		t.Errorf("comment:\n%s\nwant:\n%s", w[0].Stdin, want)
	}
	if strings.Contains(stdout.String(), "::warning::") {
		t.Errorf("a badge not published yet is expected, not a warning: %q", stdout.String())
	}
}

func TestPRWithUnreadableMainBadgeWarnsAndCommentsWithoutDiff(t *testing.T) {
	calls := fakeGhEnv(t, "no-comments.jsonl")
	getenv, _ := ciEnv(t)
	var stdout bytes.Buffer

	if err := run(prArgs(mainBadge(t, "<html>")), getenv, &stdout); err != nil {
		t.Fatal(err)
	}

	if w := calls(); len(w) != 1 || w[0].Stdin != golden(t, "comment-no-main.golden") {
		t.Errorf("want the comment without diff, got %+v", w)
	}
	if !strings.Contains(stdout.String(), "::warning::cobertura de main") {
		t.Errorf("want a warning, got %q", stdout.String())
	}
}

func TestCommentThatCannotBePublishedWarnsWithoutFailing(t *testing.T) {
	fakeGhEnv(t, "no-comments.jsonl")
	t.Setenv("FAKE_GH_FAIL", "1")
	getenv, summary := ciEnv(t)
	var stdout bytes.Buffer

	if err := run(prArgs(mainBadge(t, main713)), getenv, &stdout); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if !strings.Contains(stdout.String(), "::warning::no se pudo publicar el comentario de cobertura") {
		t.Errorf("want a warning, got %q", stdout.String())
	}
	if _, err := os.Stat(summary); err != nil {
		t.Errorf("the summary is still written: %v", err)
	}
}

func TestFailureWithoutLocationIsAnnotatedWithoutFile(t *testing.T) {
	junit := filepath.Join(t.TempDir(), "junit.xml")
	err := os.WriteFile(junit, []byte(`<testsuites tests="1" failures="1" time="600.1">
	<testsuite tests="1" failures="1" name="example.com/demo">
		<testcase classname="example.com/demo" name="TestSlow" time="600.0">
			<failure message="Failed" type="">=== RUN   TestSlow&#xA;panic: test timed out after 10m0s&#xA;</failure>
		</testcase>
	</testsuite>
</testsuites>`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	getenv, _ := ciEnv(t)
	var stdout bytes.Buffer

	args := []string{"--junit", junit, "--coverprofile", filepath.Join("testdata", "cover.out"), "--module", module}
	if err := run(args, getenv, &stdout); err != nil {
		t.Fatal(err)
	}

	want := "::error title=TestSlow::=== RUN   TestSlow%0Apanic: test timed out after 10m0s\n"
	if stdout.String() != want {
		t.Errorf("annotations = %q, want %q", stdout.String(), want)
	}
}
