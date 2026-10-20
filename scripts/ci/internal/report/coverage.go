// Package report reads the outputs of the test job (the JUnit file written by
// gotestsum and the Go coverage profile) and the KPI JSON published on the
// badges branch (ADR-0002). It is shared by the scripts of scripts/ci.
package report

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Coverage is the statement coverage of a profile, total and per package.
type Coverage struct {
	Packages []PackageCoverage // sorted by import path
}

// PackageCoverage is the statement coverage of one package.
type PackageCoverage struct {
	Package    string // import path
	Statements int
	Covered    int
}

// Percent is the share of covered statements, 0 to 100.
func (p PackageCoverage) Percent() float64 { return percent(p.Covered, p.Statements) }

// Statements is the number of statements in the profile.
func (c Coverage) Statements() int {
	n := 0
	for _, p := range c.Packages {
		n += p.Statements
	}
	return n
}

// Covered is the number of statements run at least once.
func (c Coverage) Covered() int {
	n := 0
	for _, p := range c.Packages {
		n += p.Covered
	}
	return n
}

// Percent is the total share of covered statements, 0 to 100, computed as
// `go test -cover` does per package ("coverage: N% of statements"). It can
// differ slightly from the total of `go tool cover -func`, which counts only
// the statements inside function declarations.
func (c Coverage) Percent() float64 { return percent(c.Covered(), c.Statements()) }

func percent(covered, statements int) float64 {
	if statements == 0 {
		return 0
	}
	return 100 * float64(covered) / float64(statements)
}

// ParseCoverProfile reads a profile written by `go test -coverprofile`, in any
// mode. A block listed more than once (several test binaries covering the
// same package) counts as covered if any of them ran it.
func ParseCoverProfile(r io.Reader) (Coverage, error) {
	type block struct {
		stmts   int
		covered bool
	}
	blocks := map[string]*block{} // keyed by "file:start,end"
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "mode:") {
			continue
		}
		// file.go:startLine.startCol,endLine.endCol numStmts count
		fields := strings.Fields(text)
		if len(fields) != 3 || !strings.Contains(fields[0], ":") {
			return Coverage{}, fmt.Errorf("cover profile line %d: malformed %q", line, text)
		}
		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			return Coverage{}, fmt.Errorf("cover profile line %d: malformed %q", line, text)
		}
		b := blocks[fields[0]]
		if b == nil {
			b = &block{stmts: stmts}
			blocks[fields[0]] = b
		}
		b.covered = b.covered || count > 0
	}
	if err := sc.Err(); err != nil {
		return Coverage{}, fmt.Errorf("cover profile: %w", err)
	}

	byPkg := map[string]*PackageCoverage{}
	for key, b := range blocks {
		file := key[:strings.LastIndex(key, ":")]
		pkg := path.Dir(file)
		p := byPkg[pkg]
		if p == nil {
			p = &PackageCoverage{Package: pkg}
			byPkg[pkg] = p
		}
		p.Statements += b.stmts
		if b.covered {
			p.Covered += b.stmts
		}
	}
	var cov Coverage
	for _, p := range byPkg {
		cov.Packages = append(cov.Packages, *p)
	}
	sort.Slice(cov.Packages, func(i, j int) bool { return cov.Packages[i].Package < cov.Packages[j].Package })
	return cov, nil
}
