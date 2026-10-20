package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CoverageBadge is the file name of the coverage KPI on the badges branch.
const CoverageBadge = "coverage.json"

// Badge is a KPI in the shields.io endpoint format, one JSON file per KPI on
// the orphan branch badges (ADR-0002).
type Badge struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// BadgeURL is where a KPI of the badges branch of repo ("owner/name") is
// served, the URL shields.io and the PR comment read it from.
func BadgeURL(repo, file string) string {
	return "https://raw.githubusercontent.com/" + repo + "/badges/" + file
}

// ParseBadge reads a shields.io endpoint JSON.
func ParseBadge(r io.Reader) (Badge, error) {
	var b Badge
	if err := json.NewDecoder(r).Decode(&b); err != nil {
		return Badge{}, fmt.Errorf("badge: %w", err)
	}
	if b.SchemaVersion != 1 || b.Message == "" {
		return Badge{}, errors.New("badge: not a shields.io endpoint (want schemaVersion 1 and a message)")
	}
	return b, nil
}

// FormatPercent is how coverage is written everywhere: one decimal and "%".
// A coverage badge's message is FormatPercent of the total.
func FormatPercent(p float64) string { return strconv.FormatFloat(p, 'f', 1, 64) + "%" }

// Percent reads back a message written with FormatPercent.
func (b Badge) Percent() (float64, error) {
	s, ok := strings.CutSuffix(strings.TrimSpace(b.Message), "%")
	if !ok {
		return 0, fmt.Errorf("badge: message %q is not a percentage", b.Message)
	}
	p, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("badge: message %q is not a percentage", b.Message)
	}
	return p, nil
}
