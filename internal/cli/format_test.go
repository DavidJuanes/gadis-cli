package cli

import (
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

func TestEuros(t *testing.T) {
	for in, want := range map[float64]string{
		0:       "0,00 €",
		0.79:    "0,79 €",
		10.5:    "10,50 €",
		1234.5:  "1.234,50 €",
		1234567: "1.234.567,00 €",
		-4.5:    "-4,50 €",
	} {
		if got := euros(in); got != want {
			t.Errorf("euros(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestAmount(t *testing.T) {
	for in, want := range map[float64]string{2: "2", 0.165: "0,165", 1.5: "1,5"} {
		if got := amount(in); got != want {
			t.Errorf("amount(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]float64{"3": 3, "1.5": 1.5, "1,5": 1.5, " 2 ": 2} { //nolint:gocritic // mapKey: the padded " 2 " is the input under test
		got, err := parseAmount(in)
		if err != nil || got != want {
			t.Errorf("parseAmount(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "dos", "-1"} {
		if _, err := parseAmount(in); err == nil {
			t.Errorf("parseAmount(%q) should fail", in)
		}
	}
}

func TestCheckAmount(t *testing.T) {
	unit := gadis.Product{Weight: "U"}
	byWeight := gadis.Product{Weight: "P", MinWeight: 0.8, MaxWeight: 99, Scale: 0.8}
	cases := []struct {
		p    gadis.Product
		qty  float64
		fail bool
	}{
		{unit, 3, false},
		{unit, 1.5, true},
		{byWeight, 1.6, false},
		{byWeight, 0.5, true},
		{byWeight, 120, true},
	}
	for _, c := range cases {
		if err := checkAmount(c.p, c.qty); (err != nil) != c.fail {
			t.Errorf("checkAmount(weight=%s, %v) err = %v, want fail=%v", c.p.Weight, c.qty, err, c.fail)
		}
	}
}
