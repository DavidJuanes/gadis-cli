package gadistest

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Catalog is the catalog service: product search.
type Catalog struct {
	Products []Product
}

// Product is a catalog product. It is rendered with the shape of the real
// products/search response (see docs/api.md).
type Product struct {
	ID         string
	Code       int64
	Name       string // Spanish name (commercial_description ES)
	NameGL     string // Galician name, sent first like the real API; optional
	Price      float64
	UnitPrice  float64 // price_kilo_litre
	UnitSuffix string  // e.g. "el litro"; empty renders price_kilo_litre_suffix as null
	Weight     string  // "U" by unit, "P" by weight; empty = "U"
	Scale      float64
	MinWeight  float64
	MaxWeight  float64
	Brand      string
	Slug       string
	CategoryID string
	Category   string
	// StoreIDs are the stores that sell the product; empty = every store.
	StoreIDs []string
}

func newCatalog() *Catalog {
	return &Catalog{Products: []Product{
		{
			ID: "p-leche", Code: 1001, Name: "Leche semidesnatada Gadis 1 l", NameGL: "Leite semidesnatado Gadis 1 l",
			Price: 1.17, UnitPrice: 1.17, UnitSuffix: "el litro", Brand: "GADIS",
			Slug: "/leche-semidesnatada-gadis-1-l", CategoryID: "c-leche", Category: "Leche",
		},
		{
			ID: "p-platano", Code: 2002, Name: "Plátano de Canarias", Price: 2.75, UnitPrice: 2.75,
			Weight: "P", Scale: 0.165, MinWeight: 0.165, MaxWeight: 10,
			Slug: "/platano-de-canarias", CategoryID: "c-fruta", Category: "Fruta",
		},
	}}
}

func (p Product) json() map[string]any {
	desc := []map[string]string{}
	if p.NameGL != "" {
		desc = append(desc, map[string]string{"language": "GL", "value": p.NameGL})
	}
	desc = append(desc, map[string]string{"language": "ES", "value": p.Name})
	var suffix any // null, as the real API sends for many products
	if p.UnitSuffix != "" {
		suffix = []map[string]string{{"language": "ES", "value": p.UnitSuffix}}
	}
	weight := p.Weight
	if weight == "" {
		weight = "U"
	}
	return map[string]any{
		"id": p.ID, "product_code": p.Code, "commercial_description": desc,
		"price": p.Price, "price_kilo_litre": p.UnitPrice, "price_kilo_litre_suffix": suffix,
		"weight": weight, "scale": p.Scale, "min_weight": p.MinWeight, "max_weight": p.MaxWeight,
		"brand_description": p.Brand, "slug": p.Slug,
		"category": map[string]any{
			"id": p.CategoryID, "descriptions_translate": []map[string]string{{"language": "ES", "value": p.Category}},
		},
		"image": map[string]string{}, "offers": []any{},
	}
}

func (c *Catalog) register(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("POST "+catalogPrefix+"/catalog/products/search", func(w http.ResponseWriter, r *http.Request) {
		if !requireSite(w, r) {
			return
		}
		storeID := r.Header.Get("store-id")
		if storeID == "" {
			http.Error(w, `{"message":"store-id required"}`, http.StatusBadRequest)
			return
		}
		var body struct {
			SearchTerm string `json:"search_term"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page_number"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("rows_per_page"))
		if perPage <= 0 {
			perPage = 20
		}

		defer s.lock()()
		var found []map[string]any
		term := strings.ToLower(body.SearchTerm)
		for _, p := range c.Products {
			if len(p.StoreIDs) > 0 && !slices.Contains(p.StoreIDs, storeID) {
				continue
			}
			// Like the real API, an unknown field (search_text) is ignored and
			// the whole catalog comes back.
			if term != "" && !strings.Contains(strings.ToLower(p.Name), term) {
				continue
			}
			found = append(found, p.json())
		}
		total := len(found)
		from, to := min(page*perPage, total), min((page+1)*perPage, total)
		json.NewEncoder(w).Encode(map[string]any{
			"page": map[string]int{
				"number": page, "rows_per_page": perPage,
				"total_pages": (total + perPage - 1) / perPage, "total_elements": total,
			},
			"elements": append([]map[string]any{}, found[from:to]...),
		})
	})
}
