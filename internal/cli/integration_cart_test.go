//go:build integration

// Integration tests of the account and its cart against the real
// gadisline.com. They need a session prepared beforehand with
// `GADIS_HOME=<dir> gadis login` and run with:
//
//	GADIS_TEST_HOME=<dir> go test -tags integration ./internal/cli/
//
// Without it they are skipped. The cart is the maintainer's real cart: every
// test that touches it saves its lines first and restores exactly those lines
// and amounts when it ends, whether it passes or fails (clitest.GuardCart).
// Test products come from a search and are never already in the cart.
package cli_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
)

func TestRealWhoamiAndAccountPostalCode(t *testing.T) {
	gadis := clitest.ProductionWithSession(t)

	var who struct {
		Authenticated bool   `json:"authenticated"`
		Email         string `json:"email"`
		CartID        string `json:"cart_id"`
		Delivery      struct {
			PostalCode string `json:"postal_code"`
			StoreID    string `json:"store_id"`
		} `json:"default_delivery"`
	}
	gadis.Run(t, "whoami", "--json").Success(t).JSON(t, &who)
	if !who.Authenticated || who.Email == "" || who.CartID == "" || who.Delivery.PostalCode == "" || who.Delivery.StoreID == "" {
		t.Fatalf("whoami = %+v, want an account with email, cart and default delivery", who)
	}

	// Store precedence: the account's postal code, then a saved one, then the flag.
	var out realProductList
	gadis.Run(t, "search", "leche", "--limit", "3", "--json").Success(t).JSON(t, &out)
	checkProducts(t, out.Products)
	if out.Store.Source != "account" || out.Store.PostalCode != who.Delivery.PostalCode || out.Store.ID != who.Delivery.StoreID {
		t.Errorf("search store = %+v, want the account's default delivery %+v", out.Store, who.Delivery)
	}

	gadis.Run(t, "postal-code", deliveryCP).Success(t)
	gadis.Run(t, "search", "leche", "--limit", "3", "--json").Success(t).JSON(t, &out)
	if out.Store.Source != "config" || out.Store.PostalCode != deliveryCP {
		t.Errorf("search store after `postal-code %s` = %+v, want it from the config", deliveryCP, out.Store)
	}

	gadis.Run(t, "search", "leche", "--limit", "3", "--postal-code", who.Delivery.PostalCode, "--json").Success(t).JSON(t, &out)
	if out.Store.Source != "flag" || out.Store.PostalCode != who.Delivery.PostalCode {
		t.Errorf("search store with --postal-code %s = %+v, want it from the flag", who.Delivery.PostalCode, out.Store)
	}
}

// realCart is the part of `gadis cart --json` these tests check.
type realCart struct {
	StoreID    string `json:"store_id"`
	PostalCode string `json:"postal_code"`
	Products   []struct {
		ProductID string  `json:"product_id"`
		Amount    float64 `json:"amount"`
		LinePrice float64 `json:"line_price"`
	} `json:"products"`
	ProductsPrice float64 `json:"total_product_price"`
	ShippingCost  float64 `json:"costs"`
	Total         float64 `json:"total_cart_price"`
}

// amount returns the amount of a product in the cart, 0 if it isn't there.
func (c realCart) amount(productID string) float64 {
	for _, p := range c.Products {
		if p.ProductID == productID {
			return p.Amount
		}
	}
	return 0
}

func checkTotals(t *testing.T, c realCart) {
	t.Helper()
	if math.Abs(c.Total-(c.ProductsPrice+c.ShippingCost)) > 0.01 {
		t.Errorf("total %v != products %v + shipping %v", c.Total, c.ProductsPrice, c.ShippingCost)
	}
}

// cartTest is a CLI logged in to the prepared account whose cart is restored
// when the test ends.
type cartTest struct {
	gadis *clitest.CLI
	start []clitest.CartLine // the cart's lines before the test
	cart  realCart           // the cart before the test
}

func newCartTest(t *testing.T) cartTest {
	t.Helper()
	gadis := clitest.ProductionWithSession(t)
	start := clitest.GuardCart(t, clitest.CLICart(t, gadis))
	var cart realCart
	gadis.Run(t, "cart", "--json").Success(t).JSON(t, &cart)
	return cartTest{gadis: gadis, start: start, cart: cart}
}

// cartProduct is a product in the --json output of the catalog commands.
type cartProduct struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	UnitPrice float64 `json:"unit_price"`
	Unit      string  `json:"unit"`
	ByWeight  bool    `json:"by_weight"`
	MinWeight float64 `json:"min_weight"`
	Step      float64 `json:"weight_step"`
}

// product picks, from a search in the cart's store, a product sold by weight
// or by units that isn't in the cart yet.
func (ct cartTest) product(t *testing.T, byWeight bool, terms ...string) cartProduct {
	t.Helper()
	for _, term := range terms {
		var out struct {
			Products []cartProduct `json:"products"`
		}
		ct.gadis.Run(t, "search", term, "--limit", "20", "--postal-code", ct.cart.PostalCode, "--json").Success(t).JSON(t, &out)
		for _, p := range out.Products {
			if p.ByWeight == byWeight && ct.cart.amount(p.ID) == 0 {
				return p
			}
		}
	}
	t.Fatalf("no product (by weight: %v) outside the cart in searches %v", byWeight, terms)
	return cartProduct{}
}

// checkUnchanged fails the test unless the cart has exactly its starting lines.
func (ct cartTest) checkUnchanged(t *testing.T, after string) {
	t.Helper()
	got, err := clitest.CLICart(t, ct.gadis).Lines()
	if err != nil {
		t.Fatal(err)
	}
	if !clitest.SameLines(got, ct.start) {
		t.Errorf("after %s the cart changed: %v, want %v", after, got, ct.start)
	}
}

func TestRealCartAddSetRemove(t *testing.T) {
	ct := newCartTest(t)
	p := ct.product(t, false, "leche", "agua", "aceite")

	steps := []struct {
		args []string
		want float64
	}{
		{[]string{"cart", "add", p.ID}, 1},
		{[]string{"cart", "add", p.ID, "--qty", "2"}, 3}, // add adds
		{[]string{"cart", "set", p.ID, "5"}, 5},          // set sets
		{[]string{"cart", "remove", p.ID}, 0},
	}
	for _, s := range steps {
		var cart realCart
		ct.gadis.Run(t, append(s.args, "--json")...).Success(t).JSON(t, &cart)
		if got := cart.amount(p.ID); got != s.want {
			t.Fatalf("%v: %s × %v in the cart, want %v", s.args, p.Name, got, s.want)
		}
		checkTotals(t, cart)
	}
	ct.checkUnchanged(t, "add, set and remove")
}

func TestRealCartRejectsWithoutWriting(t *testing.T) {
	ct := newCartTest(t)
	p := ct.product(t, false, "leche", "agua", "aceite")

	for _, c := range []struct {
		args []string
		err  string
	}{
		{[]string{"cart", "add", "00000000-0000-4000-8000-000000000000"}, "no existe"},
		{[]string{"cart", "add", p.ID, "--qty", "1,5"}, "por unidades"},
	} {
		res := ct.gadis.Run(t, c.args...)
		if res.Code == 0 || !strings.Contains(res.Stderr, c.err) {
			t.Errorf("%v: code %d, stderr %q; want an error with %q", c.args, res.Code, res.Stderr, c.err)
		}
		ct.checkUnchanged(t, strings.Join(c.args, " "))
	}
}

// Confirms that the amount of a product sold by weight is in kilos
// (docs/api.md, updateProduct). The log shows the line price against the
// price per kilo: line price ≈ kilos × price per kilo confirms it.
func TestRealCartByWeight(t *testing.T) {
	ct := newCartTest(t)
	p := ct.product(t, true, "plátano", "manzana", "tomate")

	// Two minimum portions: a valid amount that is a multiple of the step.
	kilos := 1.0
	switch {
	case p.MinWeight > 0:
		kilos = 2 * p.MinWeight
	case p.Step > 0:
		kilos = 2 * p.Step
	}
	qty := strconv.FormatFloat(kilos, 'f', -1, 64)

	var cart realCart
	ct.gadis.Run(t, "cart", "add", p.ID, "--qty", qty, "--json").Success(t).JSON(t, &cart)
	got := cart.amount(p.ID)
	if math.Abs(got-kilos) > 1e-6 {
		t.Errorf("added %s × %s kg: the cart has amount %v", p.Name, qty, got)
	}
	checkTotals(t, cart)
	for _, l := range cart.Products {
		if l.ProductID == p.ID {
			t.Logf("%s: amount %v, line price %.2f €, price per %s %.2f € (× amount = %.2f €)",
				p.Name, l.Amount, l.LinePrice, p.Unit, p.UnitPrice, p.UnitPrice*l.Amount)
		}
	}
}

// clear --yes is only tried on a cart that was empty at the start, so a
// failed restore can never lose the maintainer's products.
func TestRealCartClear(t *testing.T) {
	ct := newCartTest(t)
	if len(ct.start) > 0 {
		t.Skip("the cart was not empty: `cart clear --yes` is only tried on a cart empty at the start")
	}
	p := ct.product(t, false, "leche", "agua", "aceite")
	ct.gadis.Run(t, "cart", "add", p.ID).Success(t)

	if res := ct.gadis.Run(t, "cart", "clear"); res.Code == 0 {
		t.Error("cart clear without --yes succeeded")
	}
	var cart realCart
	ct.gadis.Run(t, "cart", "--json").Success(t).JSON(t, &cart)
	if cart.amount(p.ID) != 1 {
		t.Errorf("cart clear without --yes changed the cart: %+v", cart.Products)
	}

	ct.gadis.Run(t, "cart", "clear", "--yes", "--json").Success(t).JSON(t, &cart)
	if len(cart.Products) != 0 {
		t.Errorf("cart clear --yes left %+v", cart.Products)
	}
	checkTotals(t, cart)
}
