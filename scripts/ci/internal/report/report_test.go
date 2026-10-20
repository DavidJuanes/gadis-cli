package report

import (
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() }) // read-only
	return f
}

// cover.out: calc 3 of 4 statements, greet 3 of 3, notests 0 of 1.
func TestCoverProfileTotalsAndPackages(t *testing.T) {
	cov, err := ParseCoverProfile(open(t, "cover.out"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cov.Percent(); got != 75 {
		t.Errorf("total = %v, want 75", got)
	}
	want := []PackageCoverage{
		{Package: "example.com/demo/calc", Statements: 4, Covered: 3},
		{Package: "example.com/demo/greet", Statements: 3, Covered: 3},
		{Package: "example.com/demo/notests", Statements: 1, Covered: 0},
	}
	if len(cov.Packages) != len(want) {
		t.Fatalf("packages = %+v, want %+v", cov.Packages, want)
	}
	for i := range want {
		if cov.Packages[i] != want[i] {
			t.Errorf("package %d = %+v, want %+v", i, cov.Packages[i], want[i])
		}
	}
}

// junit.xml: calc has TestAdd (pass), TestDivFails (fails at calc_test.go:13)
// and TestSkipped; greet has TestHello/vacío (pass), TestHello/nombre (fails
// at greet_test.go:13) and its parent TestHello (fails, no location).
func TestJUnitCountsAndFailures(t *testing.T) {
	res, err := ParseJUnit(open(t, "junit.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed != 2 || res.Failed != 3 || res.Skipped != 1 || res.Total() != 6 {
		t.Errorf("passed/failed/skipped/total = %d/%d/%d/%d, want 2/3/1/6",
			res.Passed, res.Failed, res.Skipped, res.Total())
	}
	if res.Seconds != 0.080886 {
		t.Errorf("seconds = %v, want 0.080886", res.Seconds)
	}
	want := []Failure{
		{Package: "example.com/demo/calc", Test: "TestDivFails", File: "calc_test.go", Line: 13, Message: "bad"},
		{Package: "example.com/demo/greet", Test: "TestHello/nombre", File: "greet_test.go", Line: 13, Message: "bad"},
		{Package: "example.com/demo/greet", Test: "TestHello"},
	}
	if len(res.Failures) != len(want) {
		t.Fatalf("failures = %+v, want %+v", res.Failures, want)
	}
	for i := range want {
		got := res.Failures[i]
		got.Output = ""
		if got != want[i] {
			t.Errorf("failure %d = %+v, want %+v", i, got, want[i])
		}
	}
	if res.Failures[0].Output != "=== RUN   TestDivFails\n    calc_test.go:13: bad\n--- FAIL: TestDivFails (0.00s)\n" {
		t.Errorf("output = %q", res.Failures[0].Output)
	}
}
