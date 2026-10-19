package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// cartJSON is the cart as `--json` prints it.
type cartJSON struct {
	ID            string  `json:"id"`
	StoreID       string  `json:"store_id"`
	PostalCode    string  `json:"postal_code"`
	DeliveryType  string  `json:"delivery_type"`
	ProductsPrice float64 `json:"total_product_price"`
	Shipping      float64 `json:"costs"`
	Total         float64 `json:"total_cart_price"`
	MissingFree   float64 `json:"missing_amount_free"`
	Lines         []struct {
		ProductID string  `json:"product_id"`
		Name      string  `json:"product_name"`
		Amount    float64 `json:"amount"`
		LinePrice float64 `json:"line_price"`
	} `json:"products"`
}

func (c cartJSON) amount(t *testing.T, productID string) float64 {
	t.Helper()
	for _, l := range c.Lines {
		if l.ProductID == productID {
			return l.Amount
		}
	}
	return 0
}

// loggedIn returns a fake whose catalog sells only products (so lookups by id
// find exactly them) and a CLI logged in to it.
func loggedIn(t *testing.T, products ...gadistest.Product) (*gadistest.Server, *clitest.CLI) {
	t.Helper()
	fake := gadistest.New(t)
	if products != nil {
		fake.Catalog.Products = products
	}
	gadis := clitest.New(t, fake)
	gadis.Login(t)
	return fake, gadis
}

func product(t *testing.T, id string) gadistest.Product {
	t.Helper()
	for _, p := range gadistest.New(t).Catalog.Products {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("the fake has no product %s", id)
	return gadistest.Product{}
}

// writes returns the cart writes the fake received.
func writes(fake *gadistest.Server) []gadistest.Request {
	return fake.Requests("PUT", "/api/config/updateProduct")
}

func TestCartShowsLinesTotalsAndMissingForFreeShipping(t *testing.T) {
	_, gadis := loggedIn(t)

	for _, args := range [][]string{{"cart"}, {"cart", "get"}} {
		res := gadis.Run(t, args...).Success(t)
		for _, want := range []string{
			"p1", "2 × Yogur", "1,58 €",
			"Productos: 1,58 € · Envío: 4,50 € · Total: 6,08 €",
			"Faltan 70,42 € para el envío gratis.",
		} {
			if !strings.Contains(res.Stdout, want) {
				t.Errorf("%v: stdout lacks %q:\n%s", args, want, res.Stdout)
			}
		}
	}

	var cart cartJSON
	gadis.Run(t, "cart", "get", "--json").Success(t).JSON(t, &cart)
	if cart.ID != "cart-1" || cart.StoreID != "store-1" || cart.PostalCode != "15001" || cart.DeliveryType != "HOME_DELIVERY" {
		t.Errorf("cart = %+v", cart)
	}
	if len(cart.Lines) != 1 || cart.Lines[0].ProductID != "p1" || cart.Lines[0].Name != "Yogur" ||
		cart.Lines[0].Amount != 2 || cart.Lines[0].LinePrice != 1.58 {
		t.Errorf("lines = %+v", cart.Lines)
	}
	if cart.ProductsPrice != 1.58 || cart.Shipping != 4.5 || cart.Total != 6.08 || cart.MissingFree != 70.42 {
		t.Errorf("totals = products %v, shipping %v, total %v, missing %v", cart.ProductsPrice, cart.Shipping, cart.Total, cart.MissingFree)
	}
}

func TestCartAddSumsToTheCurrentAmountAndShowsTheCart(t *testing.T) {
	fake, gadis := loggedIn(t, product(t, "p-leche"))

	var cart cartJSON
	gadis.Run(t, "cart", "add", "p-leche", "--json").Success(t).JSON(t, &cart)
	if got := cart.amount(t, "p-leche"); got != 1 {
		t.Errorf("after add: %v of p-leche, want 1", got)
	}

	res := gadis.Run(t, "cart", "add", "p-leche", "--qty", "3").Success(t)
	if !strings.Contains(res.Stdout, "4 × Leche semidesnatada Gadis 1 l") || !strings.Contains(res.Stdout, "4,68 €") {
		t.Errorf("stdout lacks the updated cart:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "2 × Yogur") {
		t.Errorf("add changed other lines:\n%s", res.Stdout)
	}
	if w := writes(fake); len(w) != 2 || !strings.Contains(string(w[1].Body), `"amount":4`) {
		t.Errorf("writes = %d, last %s; want the final amount 4", len(w), w[len(w)-1].Body)
	}
}

func TestCartSetFixesTheFinalAmount(t *testing.T) {
	_, gadis := loggedIn(t, product(t, "p-leche"))

	var cart cartJSON
	gadis.Run(t, "cart", "set", "p-leche", "5", "--json").Success(t).JSON(t, &cart)
	if got := cart.amount(t, "p-leche"); got != 5 {
		t.Errorf("after set 5: %v", got)
	}
	gadis.Run(t, "cart", "set", "p-leche", "2", "--json").Success(t).JSON(t, &cart)
	if got := cart.amount(t, "p-leche"); got != 2 {
		t.Errorf("after set 2: %v", got)
	}
	gadis.Run(t, "cart", "set", "p-leche", "0", "--json").Success(t).JSON(t, &cart)
	if len(cart.Lines) != 1 || cart.Lines[0].ProductID != "p1" {
		t.Errorf("set 0 should remove the product: %+v", cart.Lines)
	}
}

func TestCartRemoveTakesTheProductOut(t *testing.T) {
	_, gadis := loggedIn(t)

	res := gadis.Run(t, "cart", "remove", "p1").Success(t)
	if !strings.Contains(res.Stdout, "El carrito está vacío.") {
		t.Errorf("stdout lacks the updated (empty) cart:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "quitado: Yogur") {
		t.Errorf("stderr lacks the progress message:\n%s", res.Stderr)
	}
}

func TestCartRemoveOfAProductNotInTheCartFails(t *testing.T) {
	fake, gadis := loggedIn(t)

	res := gadis.Run(t, "cart", "remove", "p-leche")
	if res.Code == 0 || !strings.Contains(res.Stderr, "no está en el carrito") {
		t.Errorf("code %d, stderr %q; want a failure saying it is not in the cart", res.Code, res.Stderr)
	}
	if w := writes(fake); len(w) != 0 {
		t.Errorf("the cart got %d writes, want none", len(w))
	}
}

func TestCartInvalidChangesFailWithoutWriting(t *testing.T) {
	elsewhere := product(t, "p-leche")
	elsewhere.StoreIDs = []string{"another-store"}

	cases := []struct {
		name    string
		catalog gadistest.Product
		args    []string
		want    string
	}{
		{"product not sold in the cart's store", elsewhere, []string{"cart", "add", "p-leche"}, "no existe en la tienda del carrito (CP 15001)"},
		{"decimals for a product sold by unit", product(t, "p-leche"), []string{"cart", "set", "p-leche", "1,5"}, "la cantidad debe ser entera"},
		{"decimals added to a product sold by unit", product(t, "p-leche"), []string{"cart", "add", "p-leche", "--qty", "0.5"}, "la cantidad debe ser entera"},
		{"below the minimum weight", product(t, "p-platano"), []string{"cart", "set", "p-platano", "0,1"}, "mínimo 0,165 kg"},
		{"above the maximum weight", product(t, "p-platano"), []string{"cart", "add", "p-platano", "--qty", "10,5"}, "máximo 10 kg"},
		{"clear without --yes", product(t, "p-leche"), []string{"cart", "clear"}, "--yes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake, gadis := loggedIn(t, tc.catalog)

			res := gadis.Run(t, tc.args...)
			if res.Code == 0 || !strings.Contains(res.Stderr, tc.want) {
				t.Errorf("code %d, stderr %q; want a failure with %q", res.Code, res.Stderr, tc.want)
			}
			if w := writes(fake); len(w) != 0 {
				t.Errorf("the cart got %d writes, want none", len(w))
			}
			var cart cartJSON
			gadis.Run(t, "cart", "--json").Success(t).JSON(t, &cart)
			if len(cart.Lines) != 1 || cart.Lines[0].ProductID != "p1" || cart.Lines[0].Amount != 2 {
				t.Errorf("the cart changed: %+v", cart.Lines)
			}
		})
	}
}

func TestCartClearYesEmptiesTheCart(t *testing.T) {
	fake, gadis := loggedIn(t)
	fake.Cart.Lines = append(fake.Cart.Lines, gadistest.CartLine{ProductID: "p2", Name: "Pan", Amount: 1, LinePrice: 0.9})

	var cart cartJSON
	gadis.Run(t, "cart", "clear", "--yes", "--json").Success(t).JSON(t, &cart)
	if len(cart.Lines) != 0 || cart.ProductsPrice != 0 {
		t.Errorf("cart after clear: %+v", cart)
	}
	if res := gadis.Run(t, "cart").Success(t); !strings.Contains(res.Stdout, "El carrito está vacío.") {
		t.Errorf("cart is not empty:\n%s", res.Stdout)
	}
}

func TestCartAmountsAcceptDecimalCommaAndPoint(t *testing.T) {
	_, gadis := loggedIn(t, product(t, "p-platano"))

	var cart cartJSON
	gadis.Run(t, "cart", "set", "p-platano", "1,5", "--json").Success(t).JSON(t, &cart)
	if got := cart.amount(t, "p-platano"); got != 1.5 {
		t.Errorf("after set 1,5: %v kg", got)
	}
	gadis.Run(t, "cart", "add", "p-platano", "--qty", "0.25", "--json").Success(t).JSON(t, &cart)
	if got := cart.amount(t, "p-platano"); got != 1.75 {
		t.Errorf("after add 0.25: %v kg, want 1.75", got)
	}
}

func TestCartWritesUseTheCartsOwnStoreAndID(t *testing.T) {
	fake, gadis := loggedIn(t, product(t, "p-leche"))
	fake.Cart.ID, fake.Cart.StoreID, fake.Cart.PostalCode = "cart-of-15100", "store-15100", "15100"

	// The catalog works with another store than the cart's.
	var search struct {
		Store struct {
			ID string `json:"store_id"`
		} `json:"store"`
	}
	gadis.Run(t, "search", "leche", "--json").Success(t).JSON(t, &search)
	if search.Store.ID == "" || search.Store.ID == "store-15100" {
		t.Fatalf("catalog store = %q, want one other than the cart's", search.Store.ID)
	}

	gadis.Run(t, "cart", "add", "p-leche").Success(t)

	lookups := fake.Requests("POST", "/catalog/products/search")
	if last := lookups[len(lookups)-1]; last.Header.Get("store-id") != "store-15100" {
		t.Errorf("product checked in store %q, want the cart's", last.Header.Get("store-id"))
	}
	w := writes(fake)
	if len(w) != 1 {
		t.Fatalf("the cart got %d writes, want 1", len(w))
	}
	var body struct {
		StoreID string  `json:"store_id"`
		CartID  string  `json:"cartId"`
		Product string  `json:"productId"`
		Amount  float64 `json:"amount"`
	}
	if err := json.Unmarshal(w[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.StoreID != "store-15100" || body.CartID != "cart-of-15100" || body.Product != "p-leche" || body.Amount != 1 {
		t.Errorf("write = %+v, want the cart's store and id", body)
	}
}
