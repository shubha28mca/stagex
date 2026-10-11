package tickets

import (
	"net/http"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller is the HTTP surface for booking. Every handler resolves the
// caller's family from the token and passes it down — no endpoint takes a
// family id from the request body, so one buyer can never act on another's
// hold or booking.
type Controller struct{ svc *Service }

// NewController builds the booking controller.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// family pulls the authenticated family id off the context.
func family(r *http.Request) (string, error) {
	id, ok := platauth.FromContext(r.Context())
	if !ok || id.AccountID == "" {
		return "", httpx.ErrUnauthorized("login required")
	}
	return id.AccountID, nil
}

// respond is the shared success/error tail used by every handler here.
func respond(w http.ResponseWriter, status int, data any, err error) {
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, status, data)
}

// SeatMap GET /api/events/{id}/seat-map — the seat picker.
func (c *Controller) SeatMap(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.SeatMap(r.Context(), r.PathValue("id"), fam)
	respond(w, http.StatusOK, out, err)
}

// Hold POST /api/tickets/hold — claim seats for the configured window.
func (c *Controller) Hold(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	var req holdRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.Hold(r.Context(), req.EventID, fam, req.SeatIDs)
	respond(w, http.StatusCreated, out, err)
}

// Release DELETE /api/tickets/hold/{groupId} — give the seats back early.
func (c *Controller) Release(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	// The event id is only used to scope the update; the group id plus the
	// family already identify the hold uniquely.
	err = c.svc.Release(r.Context(), r.URL.Query().Get("eventId"), fam, r.PathValue("groupId"))
	respond(w, http.StatusOK, map[string]bool{"released": true}, err)
}

// Quote GET /api/tickets/hold/{groupId}/quote?coupon=CODE — priced checkout.
func (c *Controller) Quote(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.Quote(r.Context(), fam, r.PathValue("groupId"), r.URL.Query().Get("coupon"))
	respond(w, http.StatusOK, out, err)
}

// CreateBooking POST /api/tickets/bookings — turn a hold into a booking.
func (c *Controller) CreateBooking(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	var req createBookingRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.CreateBooking(r.Context(), fam, req.HoldGroupID, req.CouponCode)
	respond(w, http.StatusCreated, out, err)
}

// StartPayment POST /api/tickets/bookings/{id}/pay — open an attempt.
func (c *Controller) StartPayment(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.StartPayment(r.Context(), r.PathValue("id"), fam)
	respond(w, http.StatusCreated, out, err)
}

// Confirm POST /api/tickets/bookings/{id}/confirm — settle an attempt.
func (c *Controller) Confirm(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	var req confirmRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.Confirm(r.Context(), r.PathValue("id"), fam, req.ProviderRef, req.Success)
	respond(w, http.StatusOK, out, err)
}

// List GET /api/my/bookings — booking history.
func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.Bookings(r.Context(), fam)
	respond(w, http.StatusOK, out, err)
}

// Get GET /api/my/bookings/{id} — one booking.
func (c *Controller) Get(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.Booking(r.Context(), r.PathValue("id"), fam)
	respond(w, http.StatusOK, out, err)
}

// Cancel POST /api/my/bookings/{id}/cancel — cancel with a reason.
func (c *Controller) Cancel(w http.ResponseWriter, r *http.Request) {
	fam, err := family(r)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	var req cancelRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := c.svc.Cancel(r.Context(), r.PathValue("id"), fam, req.Reason, req.Note)
	respond(w, http.StatusOK, out, err)
}

// Reasons GET /api/tickets/cancellation-reasons — the dropdown's options.
func (c *Controller) Reasons(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, c.svc.Reasons())
}
