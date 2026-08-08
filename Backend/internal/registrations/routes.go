package registrations

import "net/http"

// RegisterRoutes wires the registration endpoint (protected).
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/registrations", protect(http.HandlerFunc(c.Create)))
}
