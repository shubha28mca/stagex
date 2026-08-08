package people

import "net/http"

// RegisterRoutes wires the people endpoints. All are protected: `protect` is the
// auth middleware that requires a valid bearer token and injects the identity.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/people", protect(http.HandlerFunc(c.List)))
	mux.Handle("POST /api/people", protect(http.HandlerFunc(c.Create)))
	mux.Handle("PATCH /api/people/{id}", protect(http.HandlerFunc(c.Update)))
	mux.Handle("DELETE /api/people/{id}", protect(http.HandlerFunc(c.Delete)))
}
