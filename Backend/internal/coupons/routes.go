package coupons

import "net/http"

// RegisterRoutes wires the coupon endpoints. Validation is public so the price
// preview works before the user commits to registering.
func RegisterRoutes(mux *http.ServeMux, c *Controller) {
	mux.HandleFunc("POST /api/coupons/validate", c.Validate)
}
