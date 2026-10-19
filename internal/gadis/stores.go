package gadis

import (
	"context"
	"fmt"
)

// PostalCode is a postal code with home delivery and the store that serves it.
type PostalCode struct {
	PostalCode       string  `json:"postal_code"`
	StoreID          string  `json:"store_id"`
	MinimumOrder     float64 `json:"minimum_order_quantity"`
	FreeShippingFrom float64 `json:"minimum_shipping_free"`
	ShippingCost     float64 `json:"shipping_costs"`
}

// ErrNoDelivery means Gadisline does not deliver to a postal code.
type ErrNoDelivery struct{ PostalCode string }

func (e ErrNoDelivery) Error() string {
	return fmt.Sprintf("Gadisline no reparte a domicilio en el código postal %s", e.PostalCode)
}

// PostalCodes lists every postal code with home delivery. It needs no session.
func (c *Client) PostalCodes(ctx context.Context) ([]PostalCode, error) {
	var out struct {
		Elements []PostalCode `json:"elements"`
	}
	err := c.do(ctx, request{method: "GET", url: c.URLs.Store + "/stores/postal-codes/delivery", out: &out})
	return out.Elements, err
}

// LookupPostalCode returns the delivery details of a postal code, or ErrNoDelivery.
func (c *Client) LookupPostalCode(ctx context.Context, postalCode string) (PostalCode, error) {
	all, err := c.PostalCodes(ctx)
	if err != nil {
		return PostalCode{}, err
	}
	for _, pc := range all {
		if pc.PostalCode == postalCode {
			return pc, nil
		}
	}
	return PostalCode{}, ErrNoDelivery{postalCode}
}
