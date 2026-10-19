package gadistest

import "net/http"

// Cart is the account's cart, shown on the home page (__NEXT_DATA__ →
// props.pageProps.cart) once logged in.
type Cart struct {
	ID           string
	StoreID      string
	PostalCode   string
	DeliveryType string
	Lines        []CartLine
	ShippingCost float64
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
		ShippingCost: 4.5,
	}
}

// nextData is the cart as the home page renders it. Callers hold the lock.
func (c *Cart) nextData() map[string]any {
	lines := make([]map[string]any, 0, len(c.Lines))
	var products float64
	for _, l := range c.Lines {
		lines = append(lines, map[string]any{
			"product_id": l.ProductID, "product_name": l.Name, "amount": l.Amount, "line_price": l.LinePrice,
		})
		products += l.LinePrice
	}
	return map[string]any{
		"id": c.ID, "store_id": c.StoreID, "postal_code": c.PostalCode, "delivery_type": c.DeliveryType,
		"products": lines, "total_products": len(lines), "total_product_price": products,
		"costs": c.ShippingCost, "total_cart_price": products + c.ShippingCost,
	}
}

func (c *Cart) register(mux *http.ServeMux, s *Server) {
	// PUT /api/config/updateProduct is added with the cart command tests.
}
