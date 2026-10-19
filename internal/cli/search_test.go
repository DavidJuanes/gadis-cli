package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// defaultStoreID is the website's default assortment, used without a session
// or postal code.
const defaultStoreID = "891d5c1e-a7a0-4287-9ea3-30c5703a4f63"

func TestSearchJSON(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "search", "leche", "--json").Success(t)

	var out struct {
		Store struct {
			ID     string `json:"store_id"`
			Source string `json:"source"`
		} `json:"store"`
		Page struct {
			Total int `json:"total_elements"`
		} `json:"page"`
		Products []struct {
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Price float64 `json:"price"`
			Unit  string  `json:"unit"`
		} `json:"products"`
	}
	res.JSON(t, &out)
	if out.Store.ID != defaultStoreID || out.Store.Source != "default" {
		t.Errorf("store = %+v, want the default store", out.Store)
	}
	if out.Page.Total != 1 || len(out.Products) != 1 {
		t.Fatalf("got %d of %d products, want the milk only:\n%s", len(out.Products), out.Page.Total, res.Stdout)
	}
	if p := out.Products[0]; p.ID != "p-leche" || p.Name != "Leche semidesnatada Gadis 1 l" || p.Price != 1.17 || p.Unit != "litro" {
		t.Errorf("product = %+v", p)
	}

	reqs := fake.Requests("POST", "/catalog/products/search")
	if len(reqs) != 1 {
		t.Fatalf("catalog got %d searches, want 1", len(reqs))
	}
	req := reqs[0]
	if req.Header.Get("site-id") != gadistest.SiteID || req.Header.Get("store-id") != defaultStoreID {
		t.Errorf("headers site-id=%q store-id=%q", req.Header.Get("site-id"), req.Header.Get("store-id"))
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["search_term"] != "leche" || body["minimum_should_match"] != float64(1) {
		t.Errorf("body = %v, want search_term and minimum_should_match", body)
	}
}
