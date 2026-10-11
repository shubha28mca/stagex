package tickets

import (
	"context"
	"time"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// CouponValidator is the slice of the coupons service this package needs. Taking
// an interface rather than the concrete service keeps booking testable and
// stops the two domains becoming entangled.
type CouponValidator interface {
	Validate(ctx context.Context, code string, subtotal float64, eventID string) (CouponResult, error)
}

// CouponResult is the discount decision, mirrored locally so this package does
// not import the coupons types.
type CouponResult struct {
	Valid    bool
	Discount float64
	Reason   string
}

// Service orchestrates the booking journey. The rules it applies live in
// logic.go; the transactions live in the repository. This layer's job is
// sequencing them and supplying the clock and configuration.
type Service struct {
	repo    *Repository
	coupons CouponValidator
	// HoldTTL is how long a selection is protected — the configured window
	// ("15 minutes" by default).
	holdTTL time.Duration
	// paymentGrace extends a hold while a payment is mid-authorisation.
	paymentGrace time.Duration
	now          func() time.Time // injectable for deterministic tests
}

// NewService builds the booking service.
func NewService(repo *Repository, coupons CouponValidator,
	holdTTL, paymentGrace time.Duration) *Service {
	return &Service{
		repo: repo, coupons: coupons,
		holdTTL: holdTTL, paymentGrace: paymentGrace,
		now: time.Now,
	}
}

// SeatMap returns the seat picker for an event, from this family's viewpoint.
func (s *Service) SeatMap(ctx context.Context, eventID, familyID string) (*EventSeatMap, error) {
	m, err := s.repo.SeatMap(ctx, eventID, familyID, s.now(), s.paymentGrace)
	if err != nil {
		return nil, err
	}
	m.HoldMinutes = int(s.holdTTL.Minutes())
	return m, nil
}

// Hold claims the requested seats for this family.
//
// Conflicts come back as a 409 naming the seats, because "someone took A12" is
// something the buyer can act on where "failed" is not.
func (s *Service) Hold(ctx context.Context, eventID, familyID string, seatIDs []string) (*Hold, error) {
	if err := ValidateSeatSelection(seatIDs); err != nil {
		return nil, err
	}

	// Selling closes once the event is under way; holding seats for a show that
	// has started would only produce a booking nobody can use or cancel.
	header, err := s.repo.eventHeader(ctx, eventID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if !EventNotStarted(header.EventStart, now) {
		return nil, httpx.ErrConflict("this event has already started — tickets are no longer on sale")
	}

	groupID, err := NewGroupID()
	if err != nil {
		return nil, err
	}
	expires := HoldDeadline(now, s.holdTTL)

	seats, conflicts, err := s.repo.HoldSeats(
		ctx, eventID, familyID, seatIDs, groupID, expires, now, s.paymentGrace)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, ConflictError(conflicts)
	}

	return &Hold{
		HoldGroupID: groupID, EventID: eventID, ExpiresAt: expires,
		Seats: seats, Totals: ComputeTotals(seats, 0),
	}, nil
}

// Release drops a hold the buyer walked away from, putting the seats back on
// sale immediately instead of making everyone wait out the window.
func (s *Service) Release(ctx context.Context, eventID, familyID, groupID string) error {
	if groupID == "" {
		return httpx.ErrBadRequest("holdGroupId is required")
	}
	return s.repo.ReleaseGroup(ctx, eventID, familyID, groupID)
}

// Quote re-prices a live hold, optionally with a coupon, without committing to
// anything — this is what the checkout screen shows.
func (s *Service) Quote(ctx context.Context, familyID, groupID, couponCode string) (*Hold, error) {
	now := s.now()
	eventID, seats, expires, err := s.repo.HeldSeatsForGroup(
		ctx, familyID, groupID, now, s.paymentGrace)
	if err != nil {
		return nil, err
	}
	if len(seats) == 0 {
		return nil, httpx.ErrConflict(
			"your seat hold expired — please select your seats again")
	}

	discount := 0.0
	if couponCode != "" {
		subtotal := ComputeTotals(seats, 0).Subtotal
		res, err := s.coupons.Validate(ctx, couponCode, subtotal, eventID)
		if err != nil {
			return nil, err
		}
		if !res.Valid {
			return nil, httpx.ErrBadRequest("coupon " + couponCode + ": " + res.Reason)
		}
		discount = res.Discount
	}

	return &Hold{
		HoldGroupID: groupID, EventID: eventID, ExpiresAt: expires,
		Seats: seats, Totals: ComputeTotals(seats, discount),
	}, nil
}

// CreateBooking turns a live hold into a booking awaiting payment. The seats
// stay held — nothing is sold until the money lands.
func (s *Service) CreateBooking(ctx context.Context, familyID, groupID, couponCode string) (*Booking, error) {
	if groupID == "" {
		return nil, httpx.ErrBadRequest("holdGroupId is required")
	}
	quote, err := s.Quote(ctx, familyID, groupID, couponCode)
	if err != nil {
		return nil, err
	}

	id, err := s.repo.CreateBooking(ctx, quote.EventID, familyID, groupID,
		quote.Seats, quote.Totals, couponCode, quote.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return s.Booking(ctx, id, familyID)
}

// StartPayment opens a payment attempt for a booking.
func (s *Service) StartPayment(ctx context.Context, bookingID, familyID string) (*PaymentIntent, error) {
	return s.repo.StartPayment(ctx, bookingID, familyID, s.now())
}

// Confirm settles a payment attempt: books the seats, allows a retry, expires
// the hold, or — when the money arrived after the seats were lost — cancels
// with an offline refund owed.
func (s *Service) Confirm(ctx context.Context, bookingID, familyID, providerRef string,
	success bool) (*ConfirmResult, error) {
	if providerRef == "" {
		return nil, httpx.ErrBadRequest("providerRef is required")
	}
	return s.repo.ApplyConfirm(ctx, bookingID, familyID, providerRef, success,
		s.now(), s.paymentGrace)
}

// Bookings returns the family's booking history with cancellation eligibility
// already resolved, so the client's button state and the server's answer agree.
func (s *Service) Bookings(ctx context.Context, familyID string) ([]Booking, error) {
	list, err := s.repo.ListBookings(ctx, familyID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	for i := range list {
		s.annotate(&list[i], now)
	}
	return list, nil
}

// Booking returns one booking belonging to this family.
func (s *Service) Booking(ctx context.Context, id, familyID string) (*Booking, error) {
	b, err := s.repo.GetBooking(ctx, id, familyID)
	if err != nil {
		return nil, err
	}
	s.annotate(b, s.now())
	return b, nil
}

// annotate fills in the derived fields the client needs. CanCancel is computed
// with the same function the cancel endpoint enforces, so a button that looks
// enabled always works.
func (s *Service) annotate(b *Booking, now time.Time) {
	ok, why := CanCancel(CancelCheck{
		Status: b.Status, EventStart: b.EventStart, Total: b.Total,
	}, now)
	b.CanCancel = ok
	if !ok {
		b.CannotCancelWhy = why
	}
}

// Cancel cancels a confirmed booking with a reason, releases its seats and
// records the offline refund owed.
func (s *Service) Cancel(ctx context.Context, bookingID, familyID, reason, note string) (*CancelResult, error) {
	if err := ValidateCancelRequest(reason, note); err != nil {
		return nil, err
	}
	res, err := s.repo.Cancel(ctx, bookingID, familyID, reason, note, s.now())
	if err != nil {
		return nil, err
	}
	s.annotate(res.Booking, s.now())
	return res, nil
}

// Reasons is the closed set of cancellation reasons for the client's dropdown,
// served from the server so the two never drift apart.
func (s *Service) Reasons() []ReasonOption {
	// A fixed order keeps the dropdown stable between requests (Go randomises
	// map iteration), with "other" last because it is the fallback.
	order := []string{
		"change_of_plan", "unable_to_attend", "wrong_seats",
		"duplicate_booking", "travel_issue", "other",
	}
	out := make([]ReasonOption, 0, len(order))
	for _, code := range order {
		if label, ok := CancellationReasons[code]; ok {
			out = append(out, ReasonOption{Code: code, Label: label})
		}
	}
	return out
}

// StartSweeper runs the housekeeping pass on a ticker until ctx is cancelled.
//
// Correctness does not depend on it: every read and claim judges expiry by
// time. It exists so lapsed rows are tidied promptly rather than lingering.
func (s *Service) StartSweeper(ctx context.Context, every time.Duration,
	log interface{ Info(string, ...any) }) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				holds, bookings, err := s.repo.SweepExpired(ctx, s.now(), s.paymentGrace)
				if err == nil && (holds > 0 || bookings > 0) {
					log.Info("released expired seat holds",
						"seats", holds, "bookings", bookings)
				}
			}
		}
	}()
}
