package gadistest

import (
	"encoding/json"
	"math"
	"net/http"
	"slices"
)

// Cart is the account's cart, shown on the home page (__NEXT_DATA__ →
// props.pageProps.cart) once logged in and changed with
// PUT /api/config/updateProduct.
type Cart struct {
	ID           string
	StoreID      string
	PostalCode   string
	DeliveryType string
	Lines        []CartLine
	ShippingCost float64 // charged below FreeShippingFrom
	// FreeShippingFrom is the products total from which shipping is free.
	FreeShippingFrom float64
}

// CartLine is a product in the cart.
type CartLine struct {
	ProductID string
	Name      string
	Amount    float64 // units, or kilos for products sold by weight
	LinePrice float64
}

func newCart() *Cart {
	return &Cart{
		ID: "cart-1", StoreID: "store-1", PostalCode: "15001", DeliveryType: "HOME_DELIVERY",
		Lines:        []CartLine{{ProductID: "p1", Name: "Yogur", Amount: 2, LinePrice: 1.58}},
		ShippingCost: 4.5, FreeShippingFrom: 72,
	}
}

// cents rounds a price to cents, as the real API returns them.
func cents(v float64) float64 { return math.Round(v*100) / 100 }

// nextData is the cart as the home page and updateProduct render it. Callers
// hold the lock.
func (c *Cart) nextData() map[string]any {
	lines := make([]map[string]any, 0, len(c.Lines))
	var products float64
	for _, l := range c.Lines {
		lines = append(lines, map[string]any{
			"product_id": l.ProductID, "product_name": l.Name, "amount": l.Amount, "line_price": l.LinePrice,
		})
		products += l.LinePrice
	}
	products = cents(products)
	shipping, missing := c.ShippingCost, cents(c.FreeShippingFrom-products)
	if missing <= 0 {
		shipping, missing = 0, 0
	}
	return map[string]any{
		"id": c.ID, "store_id": c.StoreID, "postal_code": c.PostalCode, "delivery_type": c.DeliveryType,
		"products": lines, "total_products": len(lines), "total_product_price": products,
		"costs": shipping, "total_cart_price": cents(products + shipping), "missing_amount_free": missing,
	}
}

// set sets the final amount of a product (0 removes it), priced with the
// catalog. Callers hold the lock.
func (c *Cart) set(p Product, amount float64) {
	i := slices.IndexFunc(c.Lines, func(l CartLine) bool { return l.ProductID == p.ID })
	if amount == 0 {
		if i >= 0 {
			c.Lines = slices.Delete(c.Lines, i, i+1)
		}
		return
	}
	line := CartLine{ProductID: p.ID, Name: p.Name, Amount: amount, LinePrice: cents(p.Price * amount)}
	if i >= 0 {
		c.Lines[i] = line
	} else {
		c.Lines = append(c.Lines, line)
	}
}

func (c *Cart) register(mux *http.ServeMux, s *Server) {
	// The website's own route: it works with the session cookies, no token.
	mux.HandleFunc("PUT /api/config/updateProduct", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StoreID   string   `json:"store_id"`
			CartID    string   `json:"cartId"`
			ProductID string   `json:"productId"`
			Amount    *float64 `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Amount == nil || *body.Amount < 0 {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		defer s.lock()()
		if !s.Session.loggedIn(r) {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if body.CartID != c.ID || body.StoreID != c.StoreID {
			http.Error(w, `{"message":"cart not found"}`, http.StatusNotFound)
			return
		}
		p := Product{ID: body.ProductID}
		if *body.Amount > 0 {
			i := slices.IndexFunc(s.Catalog.Products, func(p Product) bool {
				return p.ID == body.ProductID && (len(p.StoreIDs) == 0 || slices.Contains(p.StoreIDs, c.StoreID))
			})
			if i < 0 {
				http.Error(w, `{"message":"product not found"}`, http.StatusNotFound)
				return
			}
			p = s.Catalog.Products[i]
		}
		c.set(p, *body.Amount)
		json.NewEncoder(w).Encode(c.nextData())
	})
}
