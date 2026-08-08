package events

import "net/http"

// RegisterRoutes wires the events endpoints onto the shared mux. Events are
// public (no auth) so participants can browse before logging in.
func RegisterRoutes(mux *http.ServeMux, c *Controller) {
	mux.HandleFunc("GET /api/events", c.List)
	mux.HandleFunc("GET /api/events/{id}", c.Get)
}
