package clitest

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// memCart is an in-memory cart that records the changes it receives.
type memCart struct {
	lines   []CartLine
	sets    []CartLine // every Set received, in order
	failSet error      // returned by Set when not nil
}

func (m *memCart) Lines() ([]CartLine, error) { return slices.Clone(m.lines), nil }

func (m *memCart) Set(productID string, amount float64) error {
	m.sets = append(m.sets, CartLine{productID, amount})
	if m.failSet != nil {
		return m.failSet
	}
	i := slices.IndexFunc(m.lines, func(l CartLine) bool { return l.ProductID == productID })
	switch {
	case i < 0 && amount > 0:
		m.lines = append(m.lines, CartLine{productID, amount})
	case i >= 0 && amount > 0:
		m.lines[i].Amount = amount
	case i >= 0:
		m.lines = slices.Delete(m.lines, i, i+1)
	}
	return nil
}

func sameLines(a, b []CartLine) bool {
	sorted := func(ls []CartLine) []CartLine {
		ls = slices.Clone(ls)
		slices.SortFunc(ls, func(x, y CartLine) int { return strings.Compare(x.ProductID, y.ProductID) })
		return ls
	}
	return slices.Equal(sorted(a), sorted(b))
}

// The guard restores the starting lines even when the test stops half way
// (FailNow, Fatal and SkipNow all end the test the same way, with Goexit).
func TestGuardCartRestoresWhenTestStopsHalfWay(t *testing.T) {
	start := []CartLine{{"milk", 6}, {"bananas", 0.165}, {"bread", 1}}
	cart := &memCart{lines: slices.Clone(start)}

	t.Run("stops half way", func(t *testing.T) {
		got := GuardCart(t, cart)
		if !sameLines(got, start) {
			t.Fatalf("GuardCart returned %v, want the starting lines %v", got, start)
		}
		cart.Set("test-product", 3) // added by the test
		cart.Set("milk", 2)         // changed
		cart.Set("bread", 0)        // removed
		t.SkipNow()
		t.Error("not reached")
	})

	if !sameLines(cart.lines, start) {
		t.Errorf("cart after the test = %v, want %v", cart.lines, start)
	}
}

// Restoring only touches the lines that changed: unchanged lines of the
// maintainer's cart get no writes.
func TestGuardCartOnlyWritesChangedLines(t *testing.T) {
	cart := &memCart{lines: []CartLine{{"milk", 6}, {"bread", 1}}}

	t.Run("test", func(t *testing.T) {
		GuardCart(t, cart)
		cart.Set("test-product", 1)
		cart.sets = nil
	})

	want := []CartLine{{"test-product", 0}}
	if !slices.Equal(cart.sets, want) {
		t.Errorf("restore wrote %v, want %v", cart.sets, want)
	}
}

func TestGuardCartRestoresAnEmptyCart(t *testing.T) {
	cart := &memCart{}

	t.Run("test", func(t *testing.T) {
		if got := GuardCart(t, cart); len(got) != 0 {
			t.Errorf("GuardCart = %v, want no lines", got)
		}
		cart.Set("a", 1)
		cart.Set("b", 0.5)
	})

	if len(cart.lines) != 0 {
		t.Errorf("cart after the test = %v, want it empty", cart.lines)
	}
}

// When the cart can't be restored, the error says what the cart had, so the
// maintainer can fix it by hand.
func TestRestoreCartReportsWhatTheCartHad(t *testing.T) {
	cart := &memCart{lines: []CartLine{{"test-product", 2}}, failSet: errors.New("boom")}

	err := restoreCart(cart, []CartLine{{"milk", 6}})
	if err == nil {
		t.Fatal("restoreCart succeeded with a cart that rejects every change")
	}
	for _, want := range []string{"milk", "6", "boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A backend that accepts a change but doesn't apply it is caught by reading
// the cart back.
func TestRestoreCartChecksTheResult(t *testing.T) {
	cart := &ignoringCart{memCart{lines: []CartLine{{"test-product", 2}}}}

	if err := restoreCart(cart, nil); err == nil {
		t.Fatal("restoreCart succeeded but the cart still has the test product")
	}
}

type ignoringCart struct{ memCart }

func (c *ignoringCart) Set(string, float64) error { return nil }

// The guard reads the real CLI's cart as `gadis cart --json` prints it.
func TestCLICartReadsTheAccountCart(t *testing.T) {
	fake := gadistest.New(t)
	fake.Cart.Lines = []gadistest.CartLine{
		{ProductID: "p1", Name: "Yogur", Amount: 2, LinePrice: 1.58},
		{ProductID: "p2", Name: "Plátano", Amount: 0.165, LinePrice: 0.4},
	}
	gadis := New(t, fake)
	gadis.Login(t)

	got, err := CLICart(t, gadis).Lines()
	if err != nil {
		t.Fatal(err)
	}
	if want := []CartLine{{"p1", 2}, {"p2", 0.165}}; !slices.Equal(got, want) {
		t.Errorf("Lines() = %v, want %v", got, want)
	}
}
