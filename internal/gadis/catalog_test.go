package gadis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchRequest(t *testing.T) {
	var gotBody map[string]any
	var gotHeader http.Header
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/catalog/products/search" {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, 404)
			return
		}
		gotHeader, gotQuery = r.Header.Clone(), r.URL.RawQuery
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"page":{"number":0,"rows_per_page":2,"total_pages":5,"total_elements":10},
			"elements":[{"id":"p1","commercial_description":[{"language":"GL","value":"Leite"},{"language":"ES","value":"Leche"}],
			"price":1.17,"price_kilo_litre":1.17,"price_kilo_litre_suffix":[{"language":"ES","value":"el litro"}],"weight":"U"},
			{"id":"p2","commercial_description":[{"language":"ES","value":"Plátano"}],"price":2.75,"price_kilo_litre_suffix":null,"weight":"P","scale":0.165,"min_weight":0.165}]}`))
	}))
	defer srv.Close()
	c := New(&Jar{})
	c.URLs.Catalog = srv.URL

	res, err := c.Search(context.Background(), "store-1", SearchQuery{Text: "leche", PerPage: 2})
	if err != nil {
		t.Fatal(err)
	}
	if gotHeader.Get("store-id") != "store-1" || gotHeader.Get("site-id") != SiteID {
		t.Errorf("headers store-id=%q site-id=%q", gotHeader.Get("store-id"), gotHeader.Get("site-id"))
	}
	if gotBody["search_term"] != "leche" || gotBody["minimum_should_match"] != float64(1) {
		t.Errorf("body %v: want search_term (not search_text) and minimum_should_match", gotBody)
	}
	if gotQuery != "order_field=relevance&page_number=0&rows_per_page=2&sort_type=asc" {
		t.Errorf("query %q", gotQuery)
	}
	if res.Page.TotalResults != 10 || len(res.Products) != 2 {
		t.Fatalf("result %+v", res)
	}
	milk, banana := res.Products[0], res.Products[1]
	if milk.Name() != "Leche" || milk.Unit() != "litro" || milk.ByWeight() {
		t.Errorf("milk: name %q unit %q byWeight %v", milk.Name(), milk.Unit(), milk.ByWeight())
	}
	if !banana.ByWeight() || banana.Unit() != "" || banana.MinWeight != 0.165 {
		t.Errorf("banana: %+v", banana)
	}
}

func TestFindCategory(t *testing.T) {
	tree := []Category{{
		ID: "c1", Name: "Alimentación", Slug: "/alimentacion",
		Children: []Category{{
			ID: "c2", Name: "Aceites y vinagres", Slug: "/alimentacion/aceites-y-vinagres",
			Children: []Category{{ID: "c3", Name: "Aceite oliva", Slug: "/alimentacion/aceites-y-vinagres/aceite-oliva"}},
		}},
	}}
	for key, want := range map[string]string{
		"/alimentacion/aceites-y-vinagres/aceite-oliva": "c3",
		"alimentacion/aceites-y-vinagres/aceite-oliva/": "c3",
		"aceite-oliva":       "c3",
		"Aceites-y-Vinagres": "c2",
		"alimentacion":       "c1",
		"c3":                 "c3",
	} {
		got, ok := FindCategory(tree, key)
		if !ok || got.ID != want {
			t.Errorf("FindCategory(%q) = %q, %v; want %q", key, got.ID, ok, want)
		}
	}
	if _, ok := FindCategory(tree, "aceite"); ok {
		t.Error("partial slug must not match")
	}
}

func TestLookupPostalCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"elements":[{"store_id":"s-cor","postal_code":"15002","shipping_costs":4.5,"minimum_shipping_free":72}]}`))
	}))
	defer srv.Close()
	c := New(&Jar{})
	c.URLs.Store = srv.URL

	pc, err := c.LookupPostalCode(context.Background(), "15002")
	if err != nil || pc.StoreID != "s-cor" || pc.FreeShippingFrom != 72 {
		t.Errorf("15002 = %+v, %v", pc, err)
	}
	if _, err := c.LookupPostalCode(context.Background(), "28001"); err == nil {
		t.Error("28001 should have no delivery")
	} else if _, ok := err.(ErrNoDelivery); !ok {
		t.Errorf("err = %T %v, want ErrNoDelivery", err, err)
	}
}
