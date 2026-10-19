package cli_test

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// Catalog commands (search, categories, category, product) run end to end
// against the fake Gadisline.

// milks are three products matching "leche", for paging.
func milks() []gadistest.Product {
	return []gadistest.Product{
		{ID: "p-leche-1", Code: 1, Name: "Leche entera 1 l", Price: 1.05, UnitPrice: 1.05, UnitSuffix: "el litro", CategoryID: "c-leche", Category: "Leche"},
		{ID: "p-leche-2", Code: 2, Name: "Leche semidesnatada 1 l", Price: 1.10, UnitPrice: 1.10, UnitSuffix: "el litro", CategoryID: "c-leche", Category: "Leche"},
		{ID: "p-leche-3", Code: 3, Name: "Leche desnatada 1 l", Price: 1.15, UnitPrice: 1.15, UnitSuffix: "el litro", CategoryID: "c-leche", Category: "Leche"},
	}
}

func TestSearchPaging(t *testing.T) {
	fake := gadistest.New(t)
	fake.Catalog.Products = milks()
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "search", "leche", "--page", "2", "--limit", "2").Success(t)

	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	if lines[0] != "3 resultados (página 2 de 2)" {
		t.Errorf("header = %q, want total and page", lines[0])
	}
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "p-leche-3 ") {
		t.Errorf("want only the third milk on page 2:\n%s", res.Stdout)
	}

	req := lastSearch(t, fake)
	q, _ := url.ParseQuery(req.Query)
	if q.Get("page_number") != "1" || q.Get("rows_per_page") != "2" {
		t.Errorf("query = %q, want page_number=1 (zero-based) and rows_per_page=2", req.Query)
	}
	body := searchBody(t, req)
	if body["search_term"] != "leche" || body["minimum_should_match"] != float64(1) {
		t.Errorf("body = %v, want search_term and minimum_should_match", body)
	}
	if _, ok := body["search_text"]; ok {
		t.Errorf("body = %v: search_text is ignored by the API, send search_term", body)
	}
}

func TestCategoriesDepth(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)

	for _, tc := range []struct {
		depth string
		want  string
	}{
		{"1", "Alimentación  /alimentacion\nFrescos  /frescos\n"},
		{"2", "Alimentación  /alimentacion\n" +
			"  Aceites y vinagres  /alimentacion/aceites-y-vinagres\n" +
			"Frescos  /frescos\n" +
			"  Fruta  /frescos/fruta\n" +
			"  Leche  /frescos/leche\n"},
		{"3", "Alimentación  /alimentacion\n" +
			"  Aceites y vinagres  /alimentacion/aceites-y-vinagres\n" +
			"    Aceite oliva  /alimentacion/aceites-y-vinagres/aceite-oliva\n" +
			"    Vinagre  /alimentacion/aceites-y-vinagres/vinagre\n" +
			"Frescos  /frescos\n" +
			"  Fruta  /frescos/fruta\n" +
			"  Leche  /frescos/leche\n"},
	} {
		res := gadis.Run(t, "categories", "--depth", tc.depth).Success(t)
		if res.Stdout != tc.want {
			t.Errorf("categories --depth %s:\n%s\nwant:\n%s", tc.depth, res.Stdout, tc.want)
		}
	}

	for depth, want := range map[string]int{"1": 1, "2": 2, "3": 3} {
		var tree []categoryNode
		gadis.Run(t, "categories", "--depth", depth, "--json").Success(t).JSON(t, &tree)
		if got := treeDepth(tree); got != want || len(tree) != 2 {
			t.Errorf("categories --depth %s --json: %d roots, %d levels; want 2 roots, %d levels", depth, len(tree), got, want)
		}
	}

	reqs := fake.Requests("GET", "/catalog/categories")
	if len(reqs) == 0 || reqs[0].Header.Get("store-id") != defaultStoreID || reqs[0].Header.Get("site-id") != gadistest.SiteID {
		t.Errorf("categories got %d requests, want them with site-id and the default store-id", len(reqs))
	}
}

func TestCategoryResolvesSlugSegmentAndID(t *testing.T) {
	for _, key := range []string{"/frescos/leche", "frescos/leche", "leche", "c-leche"} {
		t.Run(key, func(t *testing.T) {
			fake := gadistest.New(t)
			gadis := clitest.New(t, fake)

			res := gadis.Run(t, "category", key).Success(t)

			lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
			if lines[0] != "Leche (/frescos/leche)" || lines[1] != "1 resultados (página 1 de 1)" {
				t.Errorf("header:\n%s", res.Stdout)
			}
			if len(lines) != 3 || !strings.HasPrefix(lines[2], "p-leche ") {
				t.Errorf("want the milk only, not the banana:\n%s", res.Stdout)
			}
			body := searchBody(t, lastSearch(t, fake))
			if ids, _ := body["category_ids"].([]any); len(ids) != 1 || ids[0] != "c-leche" {
				t.Errorf("body = %v, want category_ids [c-leche]", body)
			}
			if _, ok := body["search_term"]; ok {
				t.Errorf("body = %v, want no search_term", body)
			}
		})
	}
}

func TestCategoryJSON(t *testing.T) {
	gadis := clitest.New(t, gadistest.New(t))

	var out struct {
		Store    struct{ Source string } `json:"store"`
		Products []struct{ ID string }   `json:"products"`
	}
	gadis.Run(t, "category", "fruta", "--json").Success(t).JSON(t, &out)

	if out.Store.Source != "default" || len(out.Products) != 1 || out.Products[0].ID != "p-platano" {
		t.Errorf("category fruta --json = %+v, want the banana and the store", out)
	}
}

func TestCategoryNotFound(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "category", "aceite")

	if res.Code != 1 || !strings.Contains(res.Stderr, `"aceite"`) || !strings.Contains(res.Stderr, "gadis categories --depth 3") {
		t.Errorf("code %d, stderr %q; want an error pointing to `gadis categories --depth 3`", res.Code, res.Stderr)
	}
	if n := len(fake.Requests("POST", "/catalog/products/search")); n != 0 {
		t.Errorf("searched %d times for an unknown category", n)
	}
}

func TestProductSeveralIDs(t *testing.T) {
	fake := gadistest.New(t)
	fake.Catalog.Products = append(fake.Catalog.Products, milks()...)
	gadis := clitest.New(t, fake)

	var out struct {
		Products []struct{ ID string } `json:"products"`
	}
	gadis.Run(t, "product", "p-leche", "p-platano", "p-leche-2", "--json").Success(t).JSON(t, &out)

	var got []string
	for _, p := range out.Products {
		got = append(got, p.ID)
	}
	slices.Sort(got) // the API picks the order
	if strings.Join(got, ",") != "p-leche,p-leche-2,p-platano" {
		t.Errorf("products = %v, want the three asked for", got)
	}
	body := searchBody(t, lastSearch(t, fake))
	ids, _ := body["product_ids"].([]any)
	if len(ids) != 3 || body["minimum_should_match"] != float64(1) {
		t.Errorf("body = %v, want the three product_ids", body)
	}
}

func TestProductNotFound(t *testing.T) {
	gadis := clitest.New(t, gadistest.New(t))

	res := gadis.Run(t, "product", "p-nope")

	if res.Code != 1 || res.Stdout != "" || !strings.Contains(res.Stderr, "producto no encontrado") {
		t.Errorf("code %d, stdout %q, stderr %q; want a clear \"not found\" error", res.Code, res.Stdout, res.Stderr)
	}
}

// oil has every field of the flat --json: brand, offer, image, unit price.
var oil = gadistest.Product{
	ID: "p-aceite", Code: 3003, Name: "Aceite de oliva virgen extra 1 l", NameGL: "Aceite de oliva virxe extra 1 l",
	Price: 8.95, UnitPrice: 8.95, UnitSuffix: "el litro", Brand: "COOSUR",
	Slug: "/aceite-de-oliva-virgen-extra-1-l", CategoryID: "c-aceite-oliva", Category: "Aceite oliva",
	Image:  "https://storage.gadisline.com/catalog/aceite",
	Offers: []gadistest.Offer{{Type: "DISCOUNT", Icon: "2ª unidad -50%", EndDate: "2026-10-31"}},
}

func TestProductJSONIsFlat(t *testing.T) {
	fake := gadistest.New(t)
	fake.Catalog.Products = append(fake.Catalog.Products, oil)
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "product", "p-aceite", "--json").Success(t)

	var out struct {
		Store    map[string]any   `json:"store"`
		Products []map[string]any `json:"products"`
	}
	res.JSON(t, &out)
	if out.Store["store_id"] != defaultStoreID || out.Store["source"] != "default" {
		t.Errorf("store = %v, want the default store and its source", out.Store)
	}
	if len(out.Products) != 1 {
		t.Fatalf("got %d products", len(out.Products))
	}
	want := map[string]any{
		"id": "p-aceite", "code": float64(3003), "name": "Aceite de oliva virgen extra 1 l", "brand": "COOSUR",
		"price": 8.95, "unit_price": 8.95, "unit": "litro", "by_weight": false, "category": "Aceite oliva",
		"offers": []any{"2ª unidad -50% hasta 2026-10-31"}, "image": "https://storage.gadisline.com/catalog/aceite",
		"url": "https://www.gadisline.com/aceite-de-oliva-virgen-extra-1-l",
	}
	assertFields(t, out.Products[0], want)
}

// unitProducts cover the price_kilo_litre_suffix quirk: a translated list or
// null, on products sold by unit and by weight.
func unitProducts() []gadistest.Product {
	return []gadistest.Product{
		{ID: "p-yogur", Code: 10, Name: "Yogur natural 4x125 g", Price: 1.20, UnitPrice: 2.40, UnitSuffix: "el kilo", CategoryID: "c-yogur", Category: "Yogures"},
		{ID: "p-pan", Code: 11, Name: "Pan de molde 450 g", Price: 1.62, UnitPrice: 3.60, CategoryID: "c-pan", Category: "Pan"}, // suffix null
		{ID: "p-platano", Code: 12, Name: "Plátano de Canarias", Price: 2.75, UnitPrice: 2.75, // suffix null
			Weight: "P", Scale: 0.165, MinWeight: 0.165, MaxWeight: 10, CategoryID: "c-fruta", Category: "Fruta"},
		{ID: "p-ternera", Code: 13, Name: "Filete de ternera", Price: 14.90, UnitPrice: 14.90, UnitSuffix: "el kilo",
			Weight: "P", Scale: 0.25, MinWeight: 0.5, MaxWeight: 5, CategoryID: "c-carne", Category: "Carne"},
	}
}

func TestUnitPricesInSearchText(t *testing.T) {
	fake := gadistest.New(t)
	fake.Catalog.Products = unitProducts()
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "search", "", "--limit", "10").Success(t)

	want := map[string]string{
		"p-yogur":   "Yogur natural 4x125 g  (2,40 €/kilo)",
		"p-pan":     "Pan de molde 450 g",
		"p-platano": "Plátano de Canarias  (por kilo)",
		"p-ternera": "Filete de ternera  (por kilo)",
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")[1:]
	if len(lines) != len(want) {
		t.Fatalf("want %d products:\n%s", len(want), res.Stdout)
	}
	for _, line := range lines {
		id, _, _ := strings.Cut(line, " ")
		if !strings.HasSuffix(line, "€  "+want[id]) {
			t.Errorf("line %q, want it to end with %q", line, want[id])
		}
	}
}

func TestUnitPricesInJSON(t *testing.T) {
	fake := gadistest.New(t)
	fake.Catalog.Products = unitProducts()
	gadis := clitest.New(t, fake)

	var out struct {
		Products []map[string]any `json:"products"`
	}
	gadis.Run(t, "product", "p-yogur", "p-pan", "p-platano", "p-ternera", "--json").Success(t).JSON(t, &out)

	want := map[string]map[string]any{
		"p-yogur": {"id": "p-yogur", "code": 10, "name": "Yogur natural 4x125 g", "price": 1.20,
			"unit_price": 2.40, "unit": "kilo", "by_weight": false, "category": "Yogures",
			"url": "https://www.gadisline.com"},
		"p-pan": {"id": "p-pan", "code": 11, "name": "Pan de molde 450 g", "price": 1.62,
			"unit_price": 3.60, "by_weight": false, "category": "Pan", "url": "https://www.gadisline.com"},
		// Sold by weight: the unit is the kilo even when the suffix is null.
		"p-platano": {"id": "p-platano", "code": 12, "name": "Plátano de Canarias", "price": 2.75,
			"unit_price": 2.75, "unit": "kilo", "by_weight": true, "min_weight": 0.165, "weight_step": 0.165,
			"category": "Fruta", "url": "https://www.gadisline.com"},
		"p-ternera": {"id": "p-ternera", "code": 13, "name": "Filete de ternera", "price": 14.90,
			"unit_price": 14.90, "unit": "kilo", "by_weight": true, "min_weight": 0.5, "weight_step": 0.25,
			"category": "Carne", "url": "https://www.gadisline.com"},
	}
	if len(out.Products) != len(want) {
		t.Fatalf("got %d products, want %d", len(out.Products), len(want))
	}
	for _, p := range out.Products {
		id, _ := p["id"].(string)
		t.Run(id, func(t *testing.T) { assertFields(t, p, want[id]) })
	}
}

func TestProductTextByWeight(t *testing.T) {
	fake := gadistest.New(t)
	fake.Catalog.Products = unitProducts()
	gadis := clitest.New(t, fake)

	res := gadis.Run(t, "product", "p-yogur", "p-ternera").Success(t)

	want := "Yogur natural 4x125 g\n" +
		"  id:        p-yogur\n" +
		"  código:    10\n" +
		"  precio:    1,20 €  (2,40 €/kilo)\n" +
		"  categoría: Yogures\n" +
		"  web:       https://www.gadisline.com\n" +
		"\n" +
		"Filete de ternera\n" +
		"  id:        p-ternera\n" +
		"  código:    13\n" +
		"  precio:    14,90 €  (por kilo)\n" +
		"  venta:     al peso; cantidad en kilos (mínimo 0,5, de 0,25 en 0,25)\n" +
		"  categoría: Carne\n" +
		"  web:       https://www.gadisline.com\n"
	if res.Stdout != want {
		t.Errorf("product text:\n%s\nwant:\n%s", res.Stdout, want)
	}
}

// assertFields checks that got has exactly the keys of want, with equal values.
func assertFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, w := range want {
		g, ok := got[k]
		gj, _ := json.Marshal(g)
		wj, _ := json.Marshal(w)
		if !ok || string(gj) != string(wj) {
			t.Errorf("%s = %s, want %s", k, gj, wj)
		}
	}
	for k, g := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected field %s = %v", k, g)
		}
	}
}

type categoryNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Slug     string         `json:"slug"`
	Children []categoryNode `json:"children"`
}

func treeDepth(tree []categoryNode) int {
	d := 0
	for _, n := range tree {
		d = max(d, 1+treeDepth(n.Children))
	}
	return d
}

func lastSearch(t *testing.T, fake *gadistest.Server) gadistest.Request {
	t.Helper()
	reqs := fake.Requests("POST", "/catalog/products/search")
	if len(reqs) == 0 {
		t.Fatal("catalog got no searches")
	}
	return reqs[len(reqs)-1]
}

func searchBody(t *testing.T, req gadistest.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("search body %q: %v", req.Body, err)
	}
	return body
}
