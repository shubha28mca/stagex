package auth

import "net/http"

// RegisterRoutes wires the public auth endpoints (no bearer token required).
func RegisterRoutes(mux *http.ServeMux, c *Controller) {
	mux.HandleFunc("POST /api/auth/otp/send", c.SendOTP)
	mux.HandleFunc("POST /api/auth/otp/verify", c.VerifyOTP)
	mux.HandleFunc("POST /api/auth/register", c.Register)
	mux.HandleFunc("POST /api/auth/login", c.Login)
}
