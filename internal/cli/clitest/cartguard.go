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
func GuardCart(t testing.TB, cart Cart) []CartLine {
	t.Helper()
	start, err := cart.Lines()
	if err != nil {
		t.Fatalf("leyendo el carrito antes del test: %v", err)
	}
	t.Cleanup(func() {
		if err := restoreCart(cart, start); err != nil {
			t.Errorf("no se pudo dejar el carrito como estaba: %v", err)
		}
	})
	return slices.Clone(start)
}

// restoreCart sets back the lines that differ from want, removes the ones
// that weren't there, and reads the cart back to check it.
func restoreCart(cart Cart, want []CartLine) error {
	wanted := fmt.Sprintf("el carrito tenía: %s", formatLines(want))
	got, err := cart.Lines()
	if err != nil {
		return fmt.Errorf("%w; %s", err, wanted)
	}
	var errs []error
	for _, c := range cartChanges(got, want) {
		if err := cart.Set(c.ProductID, c.Amount); err != nil {
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
		return fmt.Errorf("tras restaurarlo tiene %s; %s", formatLines(got), wanted)
	}
	return nil
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

func sameAmount(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func formatLines(ls []CartLine) string {
	if len(ls) == 0 {
		return "ninguna línea"
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

func (cc cliCart) Lines() ([]CartLine, error) {
	res := cc.c.Run(cc.t, "cart", "--json")
	if res.Code != 0 {
		return nil, fmt.Errorf("gadis cart: código %d: %s", res.Code, strings.TrimSpace(res.Stderr))
	}
	var cart struct {
		Products []struct {
			ProductID string  `json:"product_id"`
			Amount    float64 `json:"amount"`
		} `json:"products"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &cart); err != nil {
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
		return fmt.Errorf("código %d: %s", res.Code, strings.TrimSpace(res.Stderr))
	}
	return nil
}
