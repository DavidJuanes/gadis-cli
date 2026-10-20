package report

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// Results are the test results of a JUnit file written by gotestsum. Subtests
// count as tests of their own, as gotestsum lists them.
type Results struct {
	Passed, Failed, Skipped int
	Seconds                 float64 // wall time of the whole run
	Failures                []Failure
}

// Total is the number of tests run, skipped ones included.
func (r Results) Total() int { return r.Passed + r.Failed + r.Skipped }

// Failure is a failed test. File and Line point at the first `file.go:N:`
// line of its output, relative to the package directory; they are empty when
// the output has none (a parent test failing only through its subtests, a
// panic, a timeout).
type Failure struct {
	Package string // import path
	Test    string
	File    string
	Line    int
	Message string // text after `file.go:N:` on that line
	Output  string // full output of the test
}

type junitSuites struct {
	Time   string       `xml:"time,attr"`
	Suites []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Cases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Class   string     `xml:"classname,attr"`
	Name    string     `xml:"name,attr"`
	Failure *junitText `xml:"failure"`
	Error   *junitText `xml:"error"`
	Skipped *junitText `xml:"skipped"`
}

type junitText struct {
	Text string `xml:",chardata"`
}

// location matches the `    file_test.go:13: message` lines of testing.T.
var location = regexp.MustCompile(`(?m)^\s+([^\s:]+\.go):(\d+): ?(.*)$`)

// ParseJUnit reads a JUnit XML file written by `gotestsum --junitfile`.
func ParseJUnit(r io.Reader) (Results, error) {
	var doc junitSuites
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return Results{}, fmt.Errorf("junit: %w", err)
	}
	var res Results
	if doc.Time != "" {
		s, err := strconv.ParseFloat(doc.Time, 64)
		if err != nil {
			return Results{}, fmt.Errorf("junit: testsuites time %q: %w", doc.Time, err)
		}
		res.Seconds = s
	}
	for _, s := range doc.Suites {
		for _, c := range s.Cases {
			failure := c.Failure
			if failure == nil {
				failure = c.Error
			}
			switch {
			case failure != nil:
				res.Failed++
				res.Failures = append(res.Failures, newFailure(c, failure.Text))
			case c.Skipped != nil:
				res.Skipped++
			default:
				res.Passed++
			}
		}
	}
	return res, nil
}

func newFailure(c junitCase, output string) Failure {
	f := Failure{Package: c.Class, Test: c.Name, Output: output}
	if m := location.FindStringSubmatch(output); m != nil {
		line, err := strconv.Atoi(m[2])
		if err == nil {
			f.File, f.Line, f.Message = m[1], line, m[3]
		}
	}
	return f
}
