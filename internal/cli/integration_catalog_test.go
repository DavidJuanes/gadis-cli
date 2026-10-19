//go:build integration

// Integration tests of the catalog against the real gadisline.com. They need
// no session. Run with: go test -tags integration ./internal/cli/
//
// Assertions are about shapes and invariants, never about concrete prices or
// products, which change every day. Tests run sequentially and make few
// requests.
package cli_test

import (
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
)

// realProduct is the part of a product in the --json output these tests check.
type realProduct struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

type realProductList struct {
	Store struct {
		ID         string `json:"store_id"`
		PostalCode string `json:"postal_code"`
		Source     string `json:"source"`
	} `json:"store"`
	Page struct {
		Number int `json:"number"`
		Total  int `json:"total_elements"`
	} `json:"page"`
	Products []realProduct `json:"products"`
}

func checkProducts(t *testing.T, ps []realProduct) {
	t.Helper()
	if len(ps) == 0 {
		t.Fatal("no products")
	}
	for _, p := range ps {
		if p.ID == "" || p.Name == "" || p.Price <= 0 {
			t.Errorf("product without id, name or positive price: %+v", p)
		}
	}
}

// Stable top-level categories of the real catalog: the main aisles of a
// supermarket, unlikely to be renamed.
var stableCategories = []string{"/alimentacion", "/lacteos", "/limpieza"}

func TestRealCategories(t *testing.T) {
	gadis := clitest.Production(t)

	var tree []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Children []any  `json:"children"`
	}
	gadis.Run(t, "categories", "--depth", "1", "--json").Success(t).JSON(t, &tree)
	slugs := map[string]bool{}
	for _, c := range tree {
		if c.ID == "" || c.Name == "" || strings.Count(c.Slug, "/") != 1 {
			t.Errorf("not a top-level category: %+v", c)
		}
		if len(c.Children) > 0 {
			t.Errorf("--depth 1 returned children of %s", c.Slug)
		}
		slugs[c.Slug] = true
	}
	for _, s := range stableCategories {
		if !slugs[s] {
			t.Errorf("top-level category %s missing; got %v", s, slugs)
		}
	}
}

func TestRealCategoryBySlug(t *testing.T) {
	gadis := clitest.Production(t)

	// By its last segment and by its full slug.
	for _, slug := range []string{"lacteos", "/lacteos"} {
		var out realProductList
		gadis.Run(t, "category", slug, "--limit", "5", "--json").Success(t).JSON(t, &out)
		checkProducts(t, out.Products)
	}

	res := gadis.Run(t, "category", "no-existe-esta-categoria")
	if res.Code == 0 || !strings.Contains(res.Stderr, "no existe la categoría") {
		t.Errorf("unknown category: code %d, stderr %q", res.Code, res.Stderr)
	}
}

func TestRealProductFromSearch(t *testing.T) {
	gadis := clitest.Production(t)

	var found realProductList
	gadis.Run(t, "search", "aceite", "--limit", "1", "--json").Success(t).JSON(t, &found)
	checkProducts(t, found.Products)
	want := found.Products[0]

	var out struct {
		Products []realProduct `json:"products"`
	}
	gadis.Run(t, "product", want.ID, "--json").Success(t).JSON(t, &out)
	if len(out.Products) != 1 || out.Products[0].ID != want.ID || out.Products[0].Name != want.Name {
		t.Errorf("product %s = %+v, want %+v", want.ID, out.Products, want)
	}
	checkProducts(t, out.Products)
}

// Postal codes whose delivery status is stable: central A Coruña (Gadis's home
// ground) and central Madrid (outside Galicia and Castilla y León).
const (
	deliveryCP   = "15001"
	noDeliveryCP = "28001"
)

func TestRealPostalCodeWithDelivery(t *testing.T) {
	gadis := clitest.Production(t)

	var saved struct {
		ID         string `json:"store_id"`
		PostalCode string `json:"postal_code"`
		Source     string `json:"source"`
	}
	gadis.Run(t, "postal-code", deliveryCP, "--json").Success(t).JSON(t, &saved)
	if saved.ID == "" || saved.PostalCode != deliveryCP || saved.Source != "config" {
		t.Errorf("postal-code %s = %+v, want its store, saved", deliveryCP, saved)
	}

	var out realProductList
	gadis.Run(t, "search", "pan", "--limit", "3", "--postal-code", deliveryCP, "--json").Success(t).JSON(t, &out)
	if out.Store.ID != saved.ID || out.Store.PostalCode != deliveryCP || out.Store.Source != "flag" {
		t.Errorf("search --postal-code %s: store = %+v, want %s from the flag", deliveryCP, out.Store, saved.ID)
	}
	checkProducts(t, out.Products)
}

func TestRealPostalCodeWithoutDelivery(t *testing.T) {
	gadis := clitest.Production(t)

	for _, args := range [][]string{
		{"postal-code", noDeliveryCP},
		{"search", "pan", "--postal-code", noDeliveryCP},
	} {
		res := gadis.Run(t, args...)
		if res.Code == 0 || !strings.Contains(res.Stderr, "no reparte") {
			t.Errorf("%v: code %d, stderr %q; want a no-delivery error", args, res.Code, res.Stderr)
		}
	}
}

func TestRealSearchPaginates(t *testing.T) {
	gadis := clitest.Production(t)

	var first, second realProductList
	gadis.Run(t, "search", "leche", "--limit", "5", "--json").Success(t).JSON(t, &first)
	checkProducts(t, first.Products)
	if first.Page.Total <= 5 {
		t.Fatalf("only %d results for a common term, can't check paging", first.Page.Total)
	}

	gadis.Run(t, "search", "leche", "--limit", "5", "--page", "2", "--json").Success(t).JSON(t, &second)
	checkProducts(t, second.Products)
	if second.Page.Number != 1 {
		t.Errorf("page number = %d, want 1 (0-based) for --page 2", second.Page.Number)
	}
	seen := map[string]bool{}
	for _, p := range first.Products {
		seen[p.ID] = true
	}
	for _, p := range second.Products {
		if seen[p.ID] {
			t.Errorf("product %s is on pages 1 and 2", p.ID)
		}
	}
}
