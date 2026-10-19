package gadistest

import (
	"encoding/json"
	"net/http"
)

// Stores is the store service: postal codes with home delivery.
type Stores struct {
	PostalCodes []PostalCode
}

// PostalCode is a postal code with home delivery, with the shape of
// stores/postal-codes/delivery (see docs/api.md).
type PostalCode struct {
	PostalCode       string  `json:"postal_code"`
	StoreID          string  `json:"store_id"`
	MinimumOrder     float64 `json:"minimum_order_quantity"`
	FreeShippingFrom float64 `json:"minimum_shipping_free"`
	ShippingCost     float64 `json:"shipping_costs"`
}

func newStores() *Stores {
	return &Stores{PostalCodes: []PostalCode{
		{PostalCode: "15001", StoreID: "store-1", MinimumOrder: 40, FreeShippingFrom: 72, ShippingCost: 4.5},
		{PostalCode: "15002", StoreID: "store-2", MinimumOrder: 40, FreeShippingFrom: 72, ShippingCost: 4.5},
		{PostalCode: "15405", StoreID: "store-3", MinimumOrder: 40, FreeShippingFrom: 72, ShippingCost: 4.5},
	}}
}

func (st *Stores) register(mux *http.ServeMux, s *Server) {
	// Every postal code with home delivery; no cookies nor token needed.
	mux.HandleFunc("GET "+storePrefix+"/stores/postal-codes/delivery", func(w http.ResponseWriter, r *http.Request) {
		if !requireSite(w, r) {
			return
		}
		defer s.lock()()
		json.NewEncoder(w).Encode(map[string]any{"elements": append([]PostalCode{}, st.PostalCodes...)})
	})
}
