package main

import (
	"os"
	"path/filepath"
	"testing"
)

// inputs are the fixtures of a run: cover.out covers 75% of the statements,
// junit.xml has 2 passing, 3 failing and 1 skipped test.
func inputs(out string) []string {
	return []string{
		"--junit", filepath.Join("testdata", "junit.xml"),
		"--coverprofile", filepath.Join("testdata", "cover.out"),
		"--govulncheck", filepath.Join("testdata", "govulncheck-clean.json"),
		"--out", out,
	}
}

func readBadge(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWritesCoverageBadge(t *testing.T) {
	out := t.TempDir()
	if err := run(inputs(out)); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"label":"cobertura","message":"75.0%","color":"yellowgreen"}` + "\n"
	if got := readBadge(t, out, "coverage.json"); got != want {
		t.Errorf("coverage.json = %s, want %s", got, want)
	}
}

func TestTestsBadgeShowsFailures(t *testing.T) {
	out := t.TempDir()
	if err := run(inputs(out)); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"label":"tests","message":"2 pasan, 3 fallan","color":"red"}` + "\n"
	if got := readBadge(t, out, "tests.json"); got != want {
		t.Errorf("tests.json = %s, want %s", got, want)
	}
}

// junit-green.xml has 4 passing and 1 skipped test.
func TestTestsBadgeWhenAllPass(t *testing.T) {
	out := t.TempDir()
	args := append(inputs(out), "--junit", filepath.Join("testdata", "junit-green.xml"))
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"label":"tests","message":"4 pasan","color":"brightgreen"}` + "\n"
	if got := readBadge(t, out, "tests.json"); got != want {
		t.Errorf("tests.json = %s, want %s", got, want)
	}
}

func TestVulnsBadgeWhenClean(t *testing.T) {
	out := t.TempDir()
	if err := run(inputs(out)); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"label":"vulnerabilidades","message":"0","color":"brightgreen"}` + "\n"
	if got := readBadge(t, out, "vulns.json"); got != want {
		t.Errorf("vulns.json = %s, want %s", got, want)
	}
}

// govulncheck-findings.json has 4 vulnerabilities: GO-2026-0001 called
// through two symbols, GO-2026-0002 called once, GO-2026-0003 only imported
// and GO-2026-0004 only required. Only the called ones count, as in the text
// output and the exit code of govulncheck.
func TestVulnsBadgeCountsCalledVulnerabilities(t *testing.T) {
	out := t.TempDir()
	args := append(inputs(out), "--govulncheck", filepath.Join("testdata", "govulncheck-findings.json"))
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"label":"vulnerabilidades","message":"2","color":"red"}` + "\n"
	if got := readBadge(t, out, "vulns.json"); got != want {
		t.Errorf("vulns.json = %s, want %s", got, want)
	}
}

func TestMalformedGovulncheckIsAnError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "govulncheck.json")
	if err := os.WriteFile(bad, []byte("govulncheck: loading packages: exit status 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := run(append(inputs(out), "--govulncheck", bad)); err == nil {
		t.Error("want an error for a govulncheck output that is not JSON")
	}
}

// The scale documented in coverageColor: steps of 10 points from 50%.
func TestCoverageColorScale(t *testing.T) {
	for _, c := range []struct {
		percent float64
		want    string
	}{
		{100, "brightgreen"}, {90, "brightgreen"},
		{89.9, "green"}, {80, "green"},
		{79.9, "yellowgreen"}, {70, "yellowgreen"},
		{69.9, "yellow"}, {60, "yellow"},
		{59.9, "orange"}, {50, "orange"},
		{49.9, "red"}, {0, "red"},
	} {
		if got := coverageColor(c.percent); got != c.want {
			t.Errorf("coverageColor(%v) = %q, want %q", c.percent, got, c.want)
		}
	}
}
