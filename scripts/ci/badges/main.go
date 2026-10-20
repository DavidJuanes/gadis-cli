// Command badges writes the KPIs of a run on main as shields.io endpoint
// JSONs, one file per KPI, for the orphan branch badges (ADR-0002):
//
//	go run ./scripts/ci/badges --junit junit.xml --coverprofile coverage.out \
//	  --govulncheck govulncheck.json --out dir
//
// It writes coverage.json, tests.json and vulns.json in dir. Publishing them
// on the branch is up to the badges job of ci.yml.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DavidJuanes/gadis-cli/scripts/ci/internal/report"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "badges:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("badges", flag.ContinueOnError)
	junitPath := fs.String("junit", "", "JUnit file written by gotestsum")
	coverPath := fs.String("coverprofile", "", "coverage profile written by go test")
	vulnPath := fs.String("govulncheck", "", "output of govulncheck -format json")
	out := fs.String("out", "", "directory to write the badges in")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *junitPath == "" || *coverPath == "" || *vulnPath == "" || *out == "" {
		return errors.New("--junit, --coverprofile, --govulncheck and --out are required")
	}

	cov, err := report.ParseFile(*coverPath, report.ParseCoverProfile)
	if err != nil {
		return err
	}
	results, err := report.ParseFile(*junitPath, report.ParseJUnit)
	if err != nil {
		return err
	}
	vulns, err := report.ParseFile(*vulnPath, countCalledVulns)
	if err != nil {
		return err
	}
	for name, b := range map[string]report.Badge{
		report.CoverageBadge: coverageBadge(cov.Percent()),
		report.TestsBadge:    testsBadge(results),
		report.VulnsBadge:    vulnsBadge(vulns),
	} {
		if err := writeBadge(*out, name, b); err != nil {
			return err
		}
	}
	return nil
}

func vulnsBadge(n int) report.Badge {
	b := report.Badge{SchemaVersion: 1, Label: "vulnerabilidades", Message: strconv.Itoa(n), Color: "brightgreen"}
	if n > 0 {
		b.Color = "red"
	}
	return b
}

// countCalledVulns reads the stream of `govulncheck -format json` and counts
// the vulnerabilities the code calls: those with a finding whose trace starts
// at a function. Findings of an imported package or a required module that
// the code does not call are left out, as govulncheck does in its text output
// and its exit code.
func countCalledVulns(r io.Reader) (int, error) {
	type message struct {
		Config  *json.RawMessage `json:"config"`
		Finding *struct {
			OSV   string `json:"osv"`
			Trace []struct {
				Function string `json:"function"`
			} `json:"trace"`
		} `json:"finding"`
	}
	called := map[string]bool{}
	sawConfig := false
	dec := json.NewDecoder(r)
	for {
		var m message
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("govulncheck: %w", err)
		}
		sawConfig = sawConfig || m.Config != nil
		if f := m.Finding; f != nil && len(f.Trace) > 0 && f.Trace[0].Function != "" {
			called[f.OSV] = true
		}
	}
	// Without its config message the output is not from govulncheck, or was
	// cut short: 0 would be a lie.
	if !sawConfig {
		return 0, errors.New("govulncheck: no config message, not an output of govulncheck -format json")
	}
	return len(called), nil
}

// testsBadge counts the passing tests (subtests included, as gotestsum lists
// them) and, in red, the failing ones.
func testsBadge(r report.Results) report.Badge {
	b := report.Badge{SchemaVersion: 1, Label: "tests", Message: fmt.Sprintf("%d pasan", r.Passed), Color: "brightgreen"}
	if r.Failed > 0 {
		b.Message += fmt.Sprintf(", %d fallan", r.Failed)
		b.Color = "red"
	}
	return b
}

func coverageBadge(p float64) report.Badge {
	return report.Badge{SchemaVersion: 1, Label: "cobertura", Message: report.FormatPercent(p), Color: coverageColor(p)}
}

// coverageColor is the fixed scale of the coverage badge, applied to the
// percentage as the badge shows it (one decimal):
//
//	>= 90%  brightgreen
//	>= 80%  green
//	>= 70%  yellowgreen
//	>= 60%  yellow
//	>= 50%  orange
//	<  50%  red
func coverageColor(p float64) string {
	p = math.Round(p*10) / 10
	switch {
	case p >= 90:
		return "brightgreen"
	case p >= 80:
		return "green"
	case p >= 70:
		return "yellowgreen"
	case p >= 60:
		return "yellow"
	case p >= 50:
		return "orange"
	default:
		return "red"
	}
}

func writeBadge(dir, name string, b report.Badge) error {
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0o600)
}
