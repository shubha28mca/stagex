package payments

import "net/http"

// RegisterRoutes mounts the payment endpoints onto the router.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/payments/order", protect(http.HandlerFunc(c.CreateOrder)))
	mux.Handle("POST /api/payments/verify", protect(http.HandlerFunc(c.Verify)))
	mux.Handle("POST /api/payments/confirm", protect(http.HandlerFunc(c.Confirm)))
}
