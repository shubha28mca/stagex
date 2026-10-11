package tickets

import "net/http"

// RegisterRoutes wires the ticket-booking endpoints. Every one is protected:
// holding a seat, paying and cancelling are all family-scoped actions, and the
// seat picker itself needs a session so it can mark the caller's own hold as
// theirs rather than as taken.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	// The seat picker for one event.
	mux.Handle("GET /api/events/{id}/seat-map", protect(http.HandlerFunc(c.SeatMap)))

	// Holds — the short-lived exclusive claim on a selection.
	mux.Handle("POST /api/tickets/hold", protect(http.HandlerFunc(c.Hold)))
	mux.Handle("DELETE /api/tickets/hold/{groupId}", protect(http.HandlerFunc(c.Release)))
	mux.Handle("GET /api/tickets/hold/{groupId}/quote", protect(http.HandlerFunc(c.Quote)))

	// Booking and payment.
	mux.Handle("POST /api/tickets/bookings", protect(http.HandlerFunc(c.CreateBooking)))
	mux.Handle("POST /api/tickets/bookings/{id}/pay", protect(http.HandlerFunc(c.StartPayment)))
	mux.Handle("POST /api/tickets/bookings/{id}/confirm", protect(http.HandlerFunc(c.Confirm)))

	// History and cancellation.
	mux.Handle("GET /api/my/bookings", protect(http.HandlerFunc(c.List)))
	mux.Handle("GET /api/my/bookings/{id}", protect(http.HandlerFunc(c.Get)))
	mux.Handle("POST /api/my/bookings/{id}/cancel", protect(http.HandlerFunc(c.Cancel)))

	// The closed set of cancellation reasons, served so the client never keeps
	// its own copy of the list.
	mux.Handle("GET /api/tickets/cancellation-reasons", protect(http.HandlerFunc(c.Reasons)))
}
