package gadis

import (
	"context"
	"fmt"
)

// CartLine is a product in the cart.
type CartLine struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"product_name"`
	Code      int64   `json:"product_code"`
	Amount    float64 `json:"amount"` // units, or kilos for products sold by weight
	LinePrice float64 `json:"line_price"`
	Category  string  `json:"category_title"`
	Thumbnail string  `json:"thumbnail"`
}

// Cart is the account's shopping cart.
type Cart struct {
	ID              string     `json:"id"`
	StoreID         string     `json:"store_id"`
	PostalCode      string     `json:"postal_code"`
	DeliveryType    string     `json:"delivery_type"`
	Lines           []CartLine `json:"products"`
	TotalProducts   int        `json:"total_products"`
	ProductsPrice   float64    `json:"total_product_price"`
	ShippingCost    float64    `json:"costs"`
	OfferDiscount   float64    `json:"total_offer_discount"`
	Total           float64    `json:"total_cart_price"`
	MissingForFree  float64    `json:"missing_amount_free"` // left to reach free shipping
	MissingForOrder float64    `json:"missing_amount"`      // left to reach the minimum order
}

// Line returns the cart line of a product.
func (c Cart) Line(productID string) (CartLine, bool) {
	for _, l := range c.Lines {
		if l.ProductID == productID {
			return l, true
		}
	}
	return CartLine{}, false
}

// Cart returns the logged-in account's cart.
func (c *Client) Cart(ctx context.Context) (*Cart, error) {
	a, err := c.Account(ctx)
	if err != nil {
		return nil, err
	}
	if a.Cart.ID == "" {
		return nil, fmt.Errorf("la cuenta no tiene carrito")
	}
	return &a.Cart, nil
}

// SetAmount sets the final amount of a product in the cart (0 removes it)
// and returns the updated cart. It goes through the website's own route,
// which works with the session cookies; the cart service itself wants a token.
func (c *Client) SetAmount(ctx context.Context, cart *Cart, productID string, amount float64) (*Cart, error) {
	if amount < 0 {
		return nil, fmt.Errorf("cantidad negativa: %v", amount)
	}
	body := map[string]any{
		"store_id":            cart.StoreID,
		"cartId":              cart.ID,
		"productId":           productID,
		"amount":              amount,
		"preparation_mode_id": nil,
		"product_note":        nil,
		"substitution_type":   nil,
		"summaryCheckout":     false,
	}
	var out Cart
	err := c.do(ctx, request{method: "PUT", url: c.URLs.Web + "/api/config/updateProduct", body: body, out: &out})
	if err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, fmt.Errorf("Gadisline no devolvió el carrito tras actualizarlo") //nolint:staticcheck // ST1005: "Gadisline" is a proper noun
	}
	return &out, nil
}
