package clitest

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// CartLine is a product in the cart and its amount (units, or kilos for
// products sold by weight).
type CartLine struct {
	ProductID string
	Amount    float64
}

// Cart is the cart a guard protects: read its lines and set a product's final
// amount (0 removes it).
type Cart interface {
	Lines() ([]CartLine, error)
	Set(productID string, amount float64) error
}

// GuardCart saves the cart's lines and restores exactly those lines and
// amounts when the test ends, whether it passes, fails or stops half way. It
// returns the saved lines. Integration tests use it to leave the maintainer's
// real cart as it was.
//
// Limitation: with CLICart, lines are restored with `gadis cart set`, which
// validates the product like any other change. If a line that was in the cart
// before the test was changed and its product is no longer available, it
// can't be set back: the restore fails and the test reports what the cart had,
// to fix it by hand.
func GuardCart(t testing.TB, cart Cart) []CartLine {
	t.Helper()
	start, err := cart.Lines()
	if err != nil {
		t.Fatalf("reading the cart before the test: %v", err)
	}
	t.Cleanup(func() {
		if err := restoreCart(cart, start); err != nil {
			t.Errorf("could not leave the cart as it was: %v", err)
		}
	})
	return slices.Clone(start)
}

// restoreCart sets back the lines that differ from want, removes the ones
// that weren't there, and reads the cart back to check it.
//
// For a private cart (see privacyMarker) the errors never name its products or
// amounts, only how many lines it had and what failed.
func restoreCart(cart Cart, want []CartLine) error {
	private := isPrivate(cart)
	lines := formatLines
	if private {
		lines = countLines
	}
	wanted := fmt.Sprintf("the cart had: %s", lines(want))
	got, err := cart.Lines()
	if err != nil {
		return fmt.Errorf("%w; %s", err, wanted)
	}
	var errs []error
	for _, c := range cartChanges(got, want) {
		if err := cart.Set(c.ProductID, c.Amount); err != nil {
			if private {
				errs = append(errs, fmt.Errorf("setting a line: %w", err))
				continue
			}
			errs = append(errs, fmt.Errorf("%s → %s: %w", c.ProductID, formatAmount(c.Amount), err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%w; %s", err, wanted)
	}
	if got, err = cart.Lines(); err != nil {
		return fmt.Errorf("%w; %s", err, wanted)
	}
	if left := cartChanges(got, want); len(left) > 0 {
		return fmt.Errorf("after restoring it has %s; %s", lines(got), wanted)
	}
	return nil
}

// privacyMarker is a Cart whose lines are personal data (the maintainer's real
// cart): errors about it never show them.
type privacyMarker interface{ Private() bool }

func isPrivate(cart Cart) bool {
	p, ok := cart.(privacyMarker)
	return ok && p.Private()
}

func countLines(ls []CartLine) string {
	if len(ls) == 1 {
		return "1 line (hidden: personal data)"
	}
	return fmt.Sprintf("%d lines (hidden: personal data)", len(ls))
}

// cartChanges are the amounts to set to go from the lines got to want.
func cartChanges(got, want []CartLine) []CartLine {
	amounts := func(ls []CartLine) map[string]float64 {
		m := map[string]float64{}
		for _, l := range ls {
			m[l.ProductID] = l.Amount
		}
		return m
	}
	have, need := amounts(got), amounts(want)
	var out []CartLine
	for _, l := range want {
		if a, ok := have[l.ProductID]; !ok || !sameAmount(a, l.Amount) {
			out = append(out, l)
		}
	}
	for _, l := range got {
		if _, ok := need[l.ProductID]; !ok {
			out = append(out, CartLine{l.ProductID, 0})
		}
	}
	return out
}

// SameLines reports whether a and b hold the same products with the same
// amounts, in any order.
func SameLines(a, b []CartLine) bool {
	return len(a) == len(b) && len(cartChanges(a, b)) == 0
}

func sameAmount(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func formatLines(ls []CartLine) string {
	if len(ls) == 0 {
		return "no lines"
	}
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = l.ProductID + " × " + formatAmount(l.Amount)
	}
	return strings.Join(parts, ", ")
}

// CLICart is the cart of the account the CLI is logged in to, read with
// `gadis cart --json` and changed with `gadis cart set`.
func CLICart(t testing.TB, c *CLI) Cart { return cliCart{t, c} }

type cliCart struct {
	t testing.TB
	c *CLI
}

// Private reports whether the cart is personal data: that of a private CLI.
func (cc cliCart) Private() bool { return cc.c.Private }

// stderr is what a failed run printed on stderr, unless the CLI is private.
func (cc cliCart) stderr(res Result) string {
	if cc.c.Private {
		return hidden
	}
	return strings.TrimSpace(res.Stderr)
}

func (cc cliCart) Lines() ([]CartLine, error) {
	res := cc.c.Run(cc.t, "cart", "--json")
	if res.Code != 0 {
		return nil, fmt.Errorf("gadis cart: exit code %d: %s", res.Code, cc.stderr(res))
	}
	var cart struct {
		Products []struct {
			ProductID string  `json:"product_id"`
			Amount    float64 `json:"amount"`
		} `json:"products"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &cart); err != nil {
		if cc.c.Private {
			return nil, fmt.Errorf("gadis cart --json: not the expected JSON (%s)", hidden)
		}
		return nil, fmt.Errorf("gadis cart --json: %w", err)
	}
	lines := make([]CartLine, len(cart.Products))
	for i, p := range cart.Products {
		lines[i] = CartLine{p.ProductID, p.Amount}
	}
	return lines, nil
}

func (cc cliCart) Set(productID string, amount float64) error {
	res := cc.c.Run(cc.t, "cart", "set", productID, formatAmount(amount), "--json")
	if res.Code != 0 {
		return fmt.Errorf("exit code %d: %s", res.Code, cc.stderr(res))
	}
	return nil
}
