// Command test-report turns the outputs of the test job into a report: a
// table of tests and coverage in the step summary, an ::error annotation on
// the line of each failed test and, on pull requests, a single coverage
// comment compared with main.
//
// It runs after gotestsum, even when tests fail:
//
//	go run ./scripts/ci/test-report --junit junit.xml --coverprofile coverage.out [--pr N]
//
// with GH_TOKEN set for the PR comment. It fails only when it cannot read its
// inputs; the failing tests already fail the job.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/DavidJuanes/gadis-cli/scripts/ci/internal/gh"
	"github.com/DavidJuanes/gadis-cli/scripts/ci/internal/report"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "test-report:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, stdout io.Writer) error {
	fs := flag.NewFlagSet("test-report", flag.ContinueOnError)
	junitPath := fs.String("junit", "", "JUnit file written by gotestsum")
	coverPath := fs.String("coverprofile", "", "coverage profile written by go test")
	module := fs.String("module", "", "module path, to turn packages into repo paths (default: read from go.mod)")
	pr := fs.Int("pr", 0, "pull request to comment the coverage on (0: none)")
	mainBadge := fs.String("main-badge", "", "coverage badge of main (default: the badges branch of $GITHUB_REPOSITORY)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *junitPath == "" || *coverPath == "" {
		return errors.New("--junit and --coverprofile are required")
	}
	if *module == "" {
		m, err := modulePath("go.mod")
		if err != nil {
			return err
		}
		*module = m
	}

	results, err := report.ParseFile(*junitPath, report.ParseJUnit)
	if err != nil {
		return err
	}
	cov, err := report.ParseFile(*coverPath, report.ParseCoverProfile)
	if err != nil {
		return err
	}
	r := testReport{module: *module, results: results, cov: cov}

	if _, err := io.WriteString(stdout, r.annotations()); err != nil {
		return err
	}
	if p := getenv("GITHUB_STEP_SUMMARY"); p != "" {
		if err := appendFile(p, r.summary()); err != nil {
			return fmt.Errorf("step summary: %w", err)
		}
	}
	if *pr == 0 {
		return nil
	}

	repo := getenv("GITHUB_REPOSITORY")
	if repo == "" {
		return errors.New("GITHUB_REPOSITORY is not set")
	}
	if *mainBadge == "" {
		*mainBadge = report.BadgeURL(repo, report.CoverageBadge)
	}
	ctx := context.Background()
	mainCov, ok := fetchMainCoverage(ctx, *mainBadge, stdout)
	body := r.comment(mainCov, ok)
	// A PR from a fork gets a read-only token: the comment is lost, but that
	// says nothing about the tests, so it does not fail the job.
	if err := upsertComment(ctx, repo, *pr, body); err != nil {
		_, werr := fmt.Fprintf(stdout, "::warning::%s\n", escapeData("no se pudo publicar el comentario de cobertura: "+err.Error()))
		return werr
	}
	return nil
}

// marker identifies the coverage comment among the PR comments.
const marker = "<!-- test-report: cobertura -->"

// bot is the author of the comments written with GITHUB_TOKEN. Only its
// comments are edited: anyone can paste the marker in a comment of their own.
const bot = "github-actions[bot]"

// fetchMainCoverage reads main's total coverage from its badge. A badge that
// cannot be read (not published yet, raw.githubusercontent.com down) leaves
// the comment without a diff; anything but a 404 is also logged as a warning.
func fetchMainCoverage(ctx context.Context, url string, log io.Writer) (float64, bool) {
	warn := func(err error) {
		_, _ = fmt.Fprintf(log, "::warning::%s\n", escapeData("cobertura de main: "+err.Error()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		warn(err)
		return 0, false
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		warn(err)
		return 0, false
	}
	defer func() { _ = resp.Body.Close() }() // read-only
	if resp.StatusCode == http.StatusNotFound {
		return 0, false
	}
	if resp.StatusCode != http.StatusOK {
		warn(fmt.Errorf("GET %s: %s", url, resp.Status))
		return 0, false
	}
	b, err := report.ParseBadge(resp.Body)
	if err == nil {
		var p float64
		if p, err = b.Percent(); err == nil {
			return p, true
		}
	}
	warn(err)
	return 0, false
}

func (r testReport) comment(mainCov float64, haveMain bool) string {
	var b strings.Builder
	b.WriteString(marker + "\n## Cobertura\n\n")
	cur := report.FormatPercent(r.cov.Percent())
	if haveMain {
		fmt.Fprintf(&b, "**%s** de las sentencias, %s respecto a `main` (%s).\n",
			cur, diff(r.cov.Percent(), mainCov), report.FormatPercent(mainCov))
	} else {
		fmt.Fprintf(&b, "**%s** de las sentencias. Todavía no hay cobertura de `main` publicada "+
			"en la rama `badges`, así que no se calcula la diferencia.\n", cur)
	}
	b.WriteString("\n<details><summary>Por paquete</summary>\n\n")
	b.WriteString(r.coverageTable())
	b.WriteString("\n</details>\n\n")
	res := r.results
	fmt.Fprintf(&b, "Tests: %d %s, %d %s, %d %s.\n",
		res.Passed, plural(res.Passed, "pasa", "pasan"),
		res.Failed, plural(res.Failed, "falla", "fallan"),
		res.Skipped, plural(res.Skipped, "se salta", "se saltan"))
	return b.String()
}

// diff is the change in percentage points between the figures as they are
// shown, so that it always matches the two numbers in the comment.
func diff(cur, main float64) string {
	d := math.Round(cur*10)/10 - math.Round(main*10)/10
	switch {
	case math.Abs(d) < 0.05:
		return "sin cambios"
	case d > 0:
		return "+" + strconv.FormatFloat(d, 'f', 1, 64) + " puntos"
	default:
		return strconv.FormatFloat(d, 'f', 1, 64) + " puntos"
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// upsertComment edits the bot's coverage comment on the PR, or creates it.
func upsertComment(ctx context.Context, repo string, pr int, body string) error {
	out, err := gh.Run(ctx, "", "api", "--paginate",
		fmt.Sprintf("repos/%s/issues/%d/comments", repo, pr),
		"--jq", ".[] | {id: .id, user: .user.login, body: .body}")
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var c struct {
			ID   int64  `json:"id"`
			User string `json:"user"`
			Body string `json:"body"`
		}
		if err := dec.Decode(&c); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("gh api: comments: %w", err)
		}
		if c.User == bot && strings.Contains(c.Body, marker) {
			_, err := gh.Run(ctx, body, "api", "--method", "PATCH",
				fmt.Sprintf("repos/%s/issues/comments/%d", repo, c.ID), "-F", "body=@-")
			return err
		}
	}
	_, err = gh.Run(ctx, body, "api", "--method", "POST",
		fmt.Sprintf("repos/%s/issues/%d/comments", repo, pr), "-F", "body=@-")
	return err
}

func appendFile(name, text string) error {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302,G304: $GITHUB_STEP_SUMMARY, set by the runner
	if err != nil {
		return err
	}
	_, err = io.WriteString(f, text)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func modulePath(gomod string) (string, error) {
	data, err := os.ReadFile(gomod) //nolint:gosec // G304: go.mod of the checkout
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if m, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(m), `"`), nil
		}
	}
	return "", fmt.Errorf("%s: no module line", gomod)
}

type testReport struct {
	module  string
	results report.Results
	cov     report.Coverage
}

// dir is a package's directory relative to the repo root, "." for the root.
func (r testReport) dir(pkg string) string {
	if pkg == r.module {
		return "."
	}
	if rel, ok := strings.CutPrefix(pkg, r.module+"/"); ok {
		return rel
	}
	return pkg
}

// location is a failure's file relative to the repo root, or "" if unknown.
func (r testReport) location(f report.Failure) string {
	if f.File == "" {
		return ""
	}
	return path.Join(r.dir(f.Package), f.File)
}

// annotations are the workflow commands that put each failure on its line in
// the PR diff. A parent test that fails only through its subtests is left out:
// its subtests carry the annotation.
func (r testReport) annotations() string {
	var b strings.Builder
	for _, f := range r.results.Failures {
		if loc := r.location(f); loc != "" {
			fmt.Fprintf(&b, "::error file=%s,line=%d,title=%s::%s\n",
				escapeProperty(loc), f.Line, escapeProperty(f.Test), escapeData(f.Message))
			continue
		}
		if r.hasFailedSubtest(f) {
			continue
		}
		fmt.Fprintf(&b, "::error title=%s::%s\n",
			escapeProperty(f.Test), escapeData(strings.TrimSpace(f.Output)))
	}
	return b.String()
}

func (r testReport) hasFailedSubtest(parent report.Failure) bool {
	for _, f := range r.results.Failures {
		if f.Package == parent.Package && strings.HasPrefix(f.Test, parent.Test+"/") {
			return true
		}
	}
	return false
}

func (r testReport) summary() string {
	var b strings.Builder
	res := r.results
	b.WriteString("## Tests\n\n")
	b.WriteString("| Pasan | Fallan | Se saltan | Total | Duración |\n| ---: | ---: | ---: | ---: | ---: |\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %.1f s |\n", res.Passed, res.Failed, res.Skipped, res.Total(), res.Seconds)
	if len(res.Failures) > 0 {
		b.WriteString("\n### Tests que fallan\n\n| Paquete | Test | Dónde |\n| --- | --- | --- |\n")
		for _, f := range res.Failures {
			where := ""
			if loc := r.location(f); loc != "" {
				where = code(fmt.Sprintf("%s:%d", loc, f.Line))
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", code(r.dir(f.Package)), code(f.Test), where)
		}
	}
	b.WriteString("\n## Cobertura\n\n")
	b.WriteString(r.coverageTotal())
	b.WriteString("\n\n")
	b.WriteString(r.coverageTable())
	return b.String()
}

func (r testReport) coverageTotal() string {
	return fmt.Sprintf("Total: **%s** (%d de %d sentencias)",
		report.FormatPercent(r.cov.Percent()), r.cov.Covered(), r.cov.Statements())
}

func (r testReport) coverageTable() string {
	var b strings.Builder
	b.WriteString("| Paquete | Cobertura | Sentencias |\n| --- | ---: | ---: |\n")
	for _, p := range r.cov.Packages {
		fmt.Fprintf(&b, "| %s | %s | %d/%d |\n",
			code(r.dir(p.Package)), report.FormatPercent(p.Percent()), p.Covered, p.Statements)
	}
	return b.String()
}

// code is s as inline code in a table cell.
func code(s string) string { return "`" + strings.ReplaceAll(s, "|", `\|`) + "`" }

// escapeData and escapeProperty follow the escaping of workflow commands.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}
