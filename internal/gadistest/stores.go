package gadistest

import "net/http"

// Stores is the store service: postal codes with home delivery.
type Stores struct{}

func newStores() *Stores { return &Stores{} }

func (st *Stores) register(mux *http.ServeMux, s *Server) {
	// GET /stores/postal-codes/delivery is added with the postal code tests.
}
