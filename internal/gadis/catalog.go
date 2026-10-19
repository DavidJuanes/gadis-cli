package gadis

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Translated is a text in several languages ("ES", "GL").
type Translated []struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// ES returns the Spanish text, or the first one available.
func (t Translated) ES() string {
	for _, v := range t {
		if strings.EqualFold(v.Language, "ES") {
			return v.Value
		}
	}
	if len(t) > 0 {
		return t[0].Value
	}
	return ""
}

// Product is a catalog product as returned by products/search.
type Product struct {
	ID          string     `json:"id"` // UUID used by the cart
	Code        int64      `json:"product_code"`
	Description Translated `json:"commercial_description"`
	Price       float64    `json:"price"`
	UnitPrice   float64    `json:"price_kilo_litre"`
	UnitSuffix  Translated `json:"price_kilo_litre_suffix"` // "el kilo", "el litro"…; may be null
	// Weight is "U" for items sold by unit and "P" for items sold by weight,
	// whose amount is in kilos, a multiple of Scale and at least MinWeight.
	Weight    string  `json:"weight"`
	Scale     float64 `json:"scale"`
	MinWeight float64 `json:"min_weight"`
	MaxWeight float64 `json:"max_weight"`
	Brand     string  `json:"brand_description"`
	Slug      string  `json:"slug"`
	Category  struct {
		ID   string     `json:"id"`
		Name Translated `json:"descriptions_translate"`
	} `json:"category"`
	Image struct {
		Image     string `json:"image"`
		Thumbnail string `json:"image_thumbnails"`
	} `json:"image"`
	Offers []struct {
		Type    string `json:"type"`
		EndDate string `json:"end_date"`
		Icon    struct {
			Name string `json:"name"`
		} `json:"icon"`
	} `json:"offers"`
}

// Name is the Spanish product name.
func (p Product) Name() string { return p.Description.ES() }

// Unit is what UnitPrice is per, e.g. "kilo" or "litro" ("" if unknown).
func (p Product) Unit() string {
	return strings.TrimPrefix(strings.TrimPrefix(p.UnitSuffix.ES(), "el "), "la ")
}

// ByWeight reports whether the product is sold by weight (amount in kilos).
func (p Product) ByWeight() bool { return p.Weight == "P" }

// URL is the product page on gadisline.com.
func (p Product) URL() string { return Production.Web + p.Slug }

// Page is a page of results.
type Page struct {
	Number       int `json:"number"`
	RowsPerPage  int `json:"rows_per_page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_elements"`
}

// SearchResult is a page of products.
type SearchResult struct {
	Page     Page      `json:"page"`
	Products []Product `json:"elements"`
}

// SearchQuery filters products/search. Empty fields are omitted.
type SearchQuery struct {
	Text        string
	CategoryIDs []string
	ProductIDs  []string
	Page        int // zero-based
	PerPage     int // default 20
}

// Search searches the catalog of a store. It needs no session.
func (c *Client) Search(ctx context.Context, storeID string, q SearchQuery) (*SearchResult, error) {
	if q.PerPage <= 0 {
		q.PerPage = 20
	}
	body := map[string]any{"minimum_should_match": 1}
	if q.Text != "" {
		body["search_term"] = q.Text
	}
	if len(q.CategoryIDs) > 0 {
		body["category_ids"] = q.CategoryIDs
	}
	if len(q.ProductIDs) > 0 {
		body["product_ids"] = q.ProductIDs
	}
	query := url.Values{
		"page_number":   {strconv.Itoa(q.Page)},
		"rows_per_page": {strconv.Itoa(q.PerPage)},
	}
	if len(q.ProductIDs) == 0 {
		query.Set("order_field", "relevance")
		query.Set("sort_type", "asc")
	}
	var out SearchResult
	err := c.do(ctx, request{
		method: "POST", url: c.URLs.Catalog + "/catalog/products/search",
		query: query, body: body, storeID: storeID, out: &out,
	})
	return &out, err
}

// Products returns the given products, in the order the API sends them.
func (c *Client) Products(ctx context.Context, storeID string, ids []string) ([]Product, error) {
	res, err := c.Search(ctx, storeID, SearchQuery{ProductIDs: ids, PerPage: len(ids)})
	if err != nil {
		return nil, err
	}
	return res.Products, nil
}

// Category is a node of the category tree (three levels deep).
type Category struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Slug     string     `json:"slug"` // full path, e.g. /alimentacion/aceites-y-vinagres/aceite-oliva
	Children []Category `json:"children,omitempty"`
}

type rawCategory struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Nested struct {
		Categories []rawCategory `json:"categories"`
	} `json:"nested_categories"`
}

func (r rawCategory) convert() Category {
	c := Category{ID: r.ID, Name: r.Name, Slug: r.Slug}
	for _, n := range r.Nested.Categories {
		c.Children = append(c.Children, n.convert())
	}
	return c
}

// Categories returns the category tree of a store.
func (c *Client) Categories(ctx context.Context, storeID string) ([]Category, error) {
	var raw struct {
		Categories []rawCategory `json:"categories"`
	}
	err := c.do(ctx, request{
		method: "GET", url: c.URLs.Catalog + "/catalog/categories",
		query: url.Values{"language": {"ES"}}, storeID: storeID, out: &raw,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Category, 0, len(raw.Categories))
	for _, r := range raw.Categories {
		out = append(out, r.convert())
	}
	return out, nil
}

// FindCategory looks a category up by slug (with or without leading slash,
// matching the full path or just its last segment) or by ID.
func FindCategory(tree []Category, key string) (Category, bool) {
	key = strings.Trim(strings.ToLower(key), "/")
	var walk func([]Category) (Category, bool)
	walk = func(cs []Category) (Category, bool) {
		for _, c := range cs {
			slug := strings.Trim(strings.ToLower(c.Slug), "/")
			if c.ID == key || slug == key || lastSegment(slug) == key {
				return c, true
			}
			if found, ok := walk(c.Children); ok {
				return found, true
			}
		}
		return Category{}, false
	}
	return walk(tree)
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}
