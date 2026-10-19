package gadistest

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Catalog is the catalog service: product search and the category tree.
type Catalog struct {
	Products   []Product
	Categories []Category
	// IgnoreProductIDs makes search ignore product_ids, as the real API does
	// with fields it does not know, to check clients do not trust the result.
	IgnoreProductIDs bool
}

// Category is a node of the category tree. Slug is the full path, as in the
// real API (e.g. /alimentacion/aceites-y-vinagres/aceite-oliva).
type Category struct {
	ID       string
	Name     string
	Slug     string
	Children []Category
}

func (c Category) json() map[string]any {
	nested := []map[string]any{}
	for _, child := range c.Children {
		nested = append(nested, child.json())
	}
	return map[string]any{
		"id": c.ID, "name": c.Name, "slug": c.Slug, "image": nil,
		"nested_categories": map[string]any{"categories": nested, "has_more_elements": false},
	}
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
	Image      string // URL on storage.gadisline.com; empty renders an empty image object
	Offers     []Offer
	// StoreIDs are the stores that sell the product; empty = every store.
	StoreIDs []string
}

// Offer is a promotion on a product.
type Offer struct {
	Type    string // e.g. "DISCOUNT"
	Icon    string // shown name, e.g. "2ª unidad -50%"
	EndDate string // e.g. "2026-10-31"
}

// soldIn reports whether the product is sold in the store.
func (p Product) soldIn(storeID string) bool {
	return len(p.StoreIDs) == 0 || slices.Contains(p.StoreIDs, storeID)
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
	}, Categories: []Category{
		{ID: "c-alimentacion", Name: "Alimentación", Slug: "/alimentacion", Children: []Category{
			{ID: "c-aceites", Name: "Aceites y vinagres", Slug: "/alimentacion/aceites-y-vinagres", Children: []Category{
				{ID: "c-aceite-oliva", Name: "Aceite oliva", Slug: "/alimentacion/aceites-y-vinagres/aceite-oliva"},
				{ID: "c-vinagre", Name: "Vinagre", Slug: "/alimentacion/aceites-y-vinagres/vinagre"},
			}},
		}},
		{ID: "c-frescos", Name: "Frescos", Slug: "/frescos", Children: []Category{
			{ID: "c-fruta", Name: "Fruta", Slug: "/frescos/fruta"},
			{ID: "c-leche", Name: "Leche", Slug: "/frescos/leche"},
		}},
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
	image := map[string]string{}
	if p.Image != "" {
		image = map[string]string{"image": p.Image, "image_thumbnails": p.Image + "?thumbnail"}
	}
	offers := []map[string]any{}
	for _, o := range p.Offers {
		offers = append(offers, map[string]any{"type": o.Type, "end_date": o.EndDate, "icon": map[string]string{"name": o.Icon}})
	}
	return map[string]any{
		"id": p.ID, "product_code": p.Code, "commercial_description": desc,
		"price": p.Price, "price_kilo_litre": p.UnitPrice, "price_kilo_litre_suffix": suffix,
		"weight": weight, "scale": p.Scale, "min_weight": p.MinWeight, "max_weight": p.MaxWeight,
		"brand_description": p.Brand, "slug": p.Slug,
		"category": map[string]any{
			"id": p.CategoryID, "descriptions_translate": []map[string]string{{"language": "ES", "value": p.Category}},
		},
		"image": image, "offers": offers,
	}
}

func (c *Catalog) register(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET "+catalogPrefix+"/catalog/categories", func(w http.ResponseWriter, r *http.Request) {
		if !requireSite(w, r) {
			return
		}
		if r.Header.Get("store-id") == "" {
			http.Error(w, `{"message":"store-id required"}`, http.StatusBadRequest)
			return
		}
		defer s.lock()()
		cats := []map[string]any{}
		for _, cat := range c.Categories {
			cats = append(cats, cat.json())
		}
		json.NewEncoder(w).Encode(map[string]any{"categories": cats})
	})

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
			SearchTerm  string   `json:"search_term"`
			CategoryIDs []string `json:"category_ids"`
			ProductIDs  []string `json:"product_ids"`
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
			if !p.soldIn(storeID) {
				continue
			}
			// Like the real API, an unknown field (search_text) is ignored and
			// the whole catalog comes back.
			if term != "" && !strings.Contains(strings.ToLower(p.Name), term) {
				continue
			}
			if len(body.CategoryIDs) > 0 && !slices.Contains(body.CategoryIDs, p.CategoryID) {
				continue
			}
			// Unknown ids are left out without an error.
			if !c.IgnoreProductIDs && len(body.ProductIDs) > 0 && !slices.Contains(body.ProductIDs, p.ID) {
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
