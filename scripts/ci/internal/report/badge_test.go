package report

import (
	"strings"
	"testing"
)

func TestCoverageBadgeRoundTrips(t *testing.T) {
	b, err := ParseBadge(strings.NewReader(
		`{"schemaVersion":1,"label":"cobertura","message":"71.3%","color":"yellow"}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Percent()
	if err != nil {
		t.Fatal(err)
	}
	if got != 71.3 {
		t.Errorf("percent = %v, want 71.3", got)
	}
}

func TestBadgeWithoutPercentIsAnError(t *testing.T) {
	b, err := ParseBadge(strings.NewReader(`{"schemaVersion":1,"label":"tests","message":"120 passing","color":"green"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Percent(); err == nil {
		t.Error("want an error for a message that is not a percentage")
	}
}

func TestBadgeThatIsNotAShieldsEndpointIsAnError(t *testing.T) {
	for _, body := range []string{"404: Not Found", `{"label":"cobertura"}`} {
		if _, err := ParseBadge(strings.NewReader(body)); err == nil {
			t.Errorf("ParseBadge(%q): want an error", body)
		}
	}
}

func TestFormatPercentUsesOneDecimal(t *testing.T) {
	for in, want := range map[float64]string{200.0 / 3: "66.7%", 75: "75.0%", 0: "0.0%", 100: "100.0%"} {
		if got := FormatPercent(in); got != want {
			t.Errorf("FormatPercent(%v) = %q, want %q", in, got, want)
		}
	}
}

// RoundPercent is the percentage as FormatPercent shows it.
func TestRoundPercentKeepsOneDecimal(t *testing.T) {
	for in, want := range map[float64]float64{200.0 / 3: 66.7, 79.96: 80, 0.04: 0, 71.25: 71.3} {
		if got := RoundPercent(in); got != want {
			t.Errorf("RoundPercent(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestBadgeURLPointsAtTheBadgesBranch(t *testing.T) {
	got := BadgeURL("DavidJuanes/gadis-cli", CoverageBadge)
	want := "https://raw.githubusercontent.com/DavidJuanes/gadis-cli/badges/coverage.json"
	if got != want {
		t.Errorf("BadgeURL = %q, want %q", got, want)
	}
}
