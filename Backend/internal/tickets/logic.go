// Package tickets implements seat-level ticket booking for the participant
// app: pick seats from a hall's published arrangement, hold them briefly, pay,
// and later cancel with an offline refund.
//
// Layering matches every other domain package here: model → repository →
// service → controller → routes. This file is the model plus the pure rules —
// pricing, hold expiry, the concurrency verdict for a seat, cancellation
// eligibility and refund planning. They take no database and no clock of their
// own, which is why they are exhaustively unit-tested in logic_test.go.
package tickets

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// MaxSeatsPerBooking caps a single purchase. It exists to stop one family
// parking the whole front row while they decide — holds block everyone else.
const MaxSeatsPerBooking = 10

// MaxPaymentAttempts mirrors the registration payment rule: three tries, then
// the buyer must start again with a fresh hold.
const MaxPaymentAttempts = 3

// Booking statuses.
const (
	StatusPendingPayment    = "pending_payment"
	StatusPaymentProcessing = "payment_processing"
	StatusConfirmed         = "confirmed"
	StatusCancelled         = "cancelled"
	StatusExpired           = "expired"
)

// Per-event seat states as stored in event_seat_status.
const (
	SeatAvailable = "available"
	SeatHeld      = "held"
	SeatBooked    = "booked"
)

// Availability verdicts sent to the client. `held_by_you` is deliberately
// distinct from `held`: the buyer must see their own selection as theirs while
// everyone else sees it as gone.
const (
	AvailAvailable = "available"
	AvailHeldByYou = "held_by_you"
	AvailHeld      = "held"
	AvailBooked    = "booked"
)

// Refund states. Money is only ever returned by hand.
const (
	RefundNotApplicable  = "not_applicable"
	RefundPendingOffline = "pending_offline"
	RefundCompleted      = "completed"
)

// CancellationReasons is the closed set a family may choose from. A closed set
// keeps the data reportable — Ops can answer "why are people cancelling?" —
// while the free-text note carries the detail.
var CancellationReasons = map[string]string{
	"change_of_plan":    "Change of plan",
	"unable_to_attend":  "Unable to attend",
	"wrong_seats":       "Booked the wrong seats",
	"duplicate_booking": "Duplicate booking",
	"travel_issue":      "Travel or health issue",
	"other":             "Other",
}

// -----------------------------------------------------------------------------
// Money
// -----------------------------------------------------------------------------

// round2 keeps rupee amounts to two decimals. Float accumulation over dozens of
// seats drifts, and a booking total must match the sum of its printed tickets.
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// SeatPrice is one seat being priced into a booking.
type SeatPrice struct {
	SeatID    string  `json:"seatId"`
	SeatLabel string  `json:"seatLabel"`
	Price     float64 `json:"price"`
}

// Totals is the money breakdown shown at checkout and stored on the booking.
type Totals struct {
	SeatCount int     `json:"seatCount"`
	Subtotal  float64 `json:"subtotal"`
	Discount  float64 `json:"discount"`
	Total     float64 `json:"total"`
}

// ComputeTotals sums the seats and applies an already-validated discount.
//
// The discount is clamped to the subtotal: a coupon must never produce a
// negative total (which would read as money owed to the buyer).
func ComputeTotals(seats []SeatPrice, discount float64) Totals {
	subtotal := 0.0
	for _, s := range seats {
		subtotal += s.Price
	}
	subtotal = round2(subtotal)

	if discount < 0 {
		discount = 0
	}
	if discount > subtotal {
		discount = subtotal
	}
	discount = round2(discount)

	return Totals{
		SeatCount: len(seats),
		Subtotal:  subtotal,
		Discount:  discount,
		Total:     round2(subtotal - discount),
	}
}

// -----------------------------------------------------------------------------
// Holds and the concurrency verdict
// -----------------------------------------------------------------------------

// SeatState is the stored per-event state of one seat, as the availability
// rules see it. A zero SeatState (no row in event_seat_status) means available.
type SeatState struct {
	Status        string
	HeldBy        string
	HoldGroupID   string
	HoldExpiresAt time.Time
	BookingID     string
	// PaymentInFlight is true when this seat's hold belongs to a booking that
	// is mid-authorisation. Such a hold survives past HoldExpiresAt for the
	// grace window, so a buyer cannot be charged for a seat already resold.
	PaymentInFlight bool
}

// HoldDeadline is when a hold taken now should lapse.
func HoldDeadline(now time.Time, ttl time.Duration) time.Time { return now.Add(ttl) }

// EffectiveDeadline is the moment a hold really stops protecting its seats.
// For a booking mid-payment that is the deadline plus the grace window — the
// answer to "the payment is processing" edge case.
func EffectiveDeadline(expires time.Time, paymentInFlight bool, grace time.Duration) time.Time {
	if paymentInFlight {
		return expires.Add(grace)
	}
	return expires
}

// HoldActive reports whether a hold is still protecting its seats at `now`.
// Expiry is judged purely by time, so a stalled sweeper can never wedge a seat.
func HoldActive(st SeatState, now time.Time, grace time.Duration) bool {
	if st.Status != SeatHeld {
		return false
	}
	return now.Before(EffectiveDeadline(st.HoldExpiresAt, st.PaymentInFlight, grace))
}

// Availability is the verdict the client renders for one seat.
//
// This is the read side of the concurrency model, and it must agree exactly
// with what the claim transaction will allow — a seat shown as available that
// then fails to hold is a bug the buyer experiences as a lie.
func Availability(st SeatState, viewerFamilyID string, now time.Time, grace time.Duration) string {
	if st.Status == SeatBooked {
		return AvailBooked
	}
	if HoldActive(st, now, grace) {
		if st.HeldBy != "" && st.HeldBy == viewerFamilyID {
			return AvailHeldByYou
		}
		return AvailHeld
	}
	// Either never claimed, explicitly released, or the hold has lapsed. In all
	// three cases the seat is up for grabs again.
	return AvailAvailable
}

// CanClaim reports whether `family` may take this seat now. Re-taking a seat
// you already hold is allowed — that is how extending a selection works.
func CanClaim(st SeatState, familyID string, now time.Time, grace time.Duration) bool {
	switch Availability(st, familyID, now, grace) {
	case AvailAvailable, AvailHeldByYou:
		return true
	default:
		return false
	}
}

// ValidateSeatSelection checks a requested selection before any database work.
func ValidateSeatSelection(seatIDs []string) error {
	if len(seatIDs) == 0 {
		return httpx.ErrBadRequest("select at least one seat")
	}
	if len(seatIDs) > MaxSeatsPerBooking {
		return httpx.ErrBadRequest(fmt.Sprintf(
			"a single booking is limited to %d seats", MaxSeatsPerBooking))
	}
	seen := map[string]bool{}
	for _, id := range seatIDs {
		if strings.TrimSpace(id) == "" {
			return httpx.ErrBadRequest("a seat id was empty")
		}
		if seen[id] {
			return httpx.ErrBadRequest("the same seat was selected twice")
		}
		seen[id] = true
	}
	return nil
}

// SeatConflict names the seats that were lost to someone else, so the client
// can highlight exactly what to re-pick instead of a vague failure.
type SeatConflict struct {
	SeatID    string `json:"seatId"`
	SeatLabel string `json:"seatLabel"`
	Reason    string `json:"reason"` // booked | held
}

// ConflictError turns lost seats into a 409 whose message reads like something
// a human would say.
func ConflictError(conflicts []SeatConflict) error {
	if len(conflicts) == 0 {
		return nil
	}
	labels := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		labels = append(labels, c.SeatLabel)
	}
	joined := strings.Join(labels, ", ")
	if len(conflicts) == 1 {
		return httpx.ErrConflict(fmt.Sprintf(
			"seat %s was just taken by someone else — please pick another", joined))
	}
	return httpx.ErrConflict(fmt.Sprintf(
		"these seats were just taken by someone else: %s — please pick again", joined))
}

// -----------------------------------------------------------------------------
// Payment gating
// -----------------------------------------------------------------------------

// PayabilityInput is the state a payment decision is made from.
type PayabilityInput struct {
	Status        string
	HoldExpiresAt time.Time
	Attempts      int
}

// CanStartPayment reports whether a payment may be started for a booking, and
// why not when it may not. Returning the reason lets the API answer with a
// message the buyer can act on.
func CanStartPayment(in PayabilityInput, now time.Time) (bool, string) {
	switch in.Status {
	case StatusConfirmed:
		return false, "this booking is already paid"
	case StatusCancelled:
		return false, "this booking was cancelled"
	case StatusExpired:
		return false, "the seat hold expired — please select your seats again"
	case StatusPaymentProcessing:
		// Not an error: the caller should be handed the in-flight attempt rather
		// than starting a second authorisation for the same money.
		return false, "a payment is already in progress for this booking"
	}
	if in.Attempts >= MaxPaymentAttempts {
		return false, fmt.Sprintf(
			"payment failed %d times — please start again with a fresh selection", MaxPaymentAttempts)
	}
	if !now.Before(in.HoldExpiresAt) {
		return false, "the seat hold expired — please select your seats again"
	}
	return true, ""
}

// ConfirmOutcome is what confirmation decided, kept as data so the service can
// apply it in one transaction and the rule can be tested on its own.
type ConfirmOutcome struct {
	// Confirm the booking and mark its seats sold.
	Confirm bool
	// AlreadyDone means a duplicate webhook/click — return the booking as-is.
	AlreadyDone bool
	// Expire means the hold lapsed with nothing owed; release the seats.
	Expire bool
	// CancelWithRefund is the ugly edge case: the money was taken but the seats
	// are gone, so the booking is cancelled and finance owes a refund.
	CancelWithRefund bool
	// Retry means this attempt failed but the buyer may try again.
	Retry bool
	// Reason is the human-readable explanation for whichever branch was taken.
	Reason string
}

// ConfirmInput is everything confirmation needs to decide.
type ConfirmInput struct {
	Status        string
	HoldExpiresAt time.Time
	Attempts      int
	// PaymentSucceeded is the gateway's verdict for this attempt.
	PaymentSucceeded bool
	// SeatsStillHeld is false when a re-check found any seat no longer held by
	// this booking — someone else got it.
	SeatsStillHeld bool
}

// DecideConfirm resolves a payment result against the booking's state.
//
// The order of the checks is the whole point:
//
//  1. an already-confirmed booking is idempotent, never an error;
//  2. money taken + seats lost must become a refund, never a double sale;
//  3. only then is the happy path taken.
//
// A payment that lands inside the grace window still confirms, which is what
// makes a slow gateway safe.
func DecideConfirm(in ConfirmInput, now time.Time, grace time.Duration) ConfirmOutcome {
	switch in.Status {
	case StatusConfirmed:
		// Duplicate delivery of a success we already applied.
		return ConfirmOutcome{AlreadyDone: true, Reason: "this booking is already confirmed"}
	case StatusCancelled:
		return ConfirmOutcome{AlreadyDone: true, Reason: "this booking was cancelled"}
	}

	if !in.PaymentSucceeded {
		// A failure inside the window leaves the seats held so the buyer can
		// retry; out of attempts or out of time and the hold is given up.
		if in.Attempts >= MaxPaymentAttempts {
			return ConfirmOutcome{
				Expire: true,
				Reason: fmt.Sprintf("payment failed %d times — the seats have been released", MaxPaymentAttempts),
			}
		}
		if !now.Before(EffectiveDeadline(in.HoldExpiresAt, true, grace)) {
			return ConfirmOutcome{Expire: true, Reason: "payment failed and the seat hold expired"}
		}
		return ConfirmOutcome{Retry: true, Reason: "payment failed — you can try again"}
	}

	// --- the payment succeeded ---

	// Seats gone: the buyer has been charged, so this can only be unwound with
	// an offline refund. Confirming anyway would sell one seat twice.
	if !in.SeatsStillHeld {
		return ConfirmOutcome{
			CancelWithRefund: true,
			Reason: "your payment went through but the seats were released before it " +
				"completed — the booking is cancelled and the amount will be refunded offline",
		}
	}

	// Past even the grace window. The seats are still ours, so honour the sale
	// rather than punishing the buyer for a slow gateway.
	if !now.Before(EffectiveDeadline(in.HoldExpiresAt, true, grace)) {
		return ConfirmOutcome{
			Confirm: true,
			Reason:  "payment confirmed after the hold window, but the seats were still yours",
		}
	}

	return ConfirmOutcome{Confirm: true, Reason: "payment confirmed"}
}

// -----------------------------------------------------------------------------
// Cancellation and refunds
// -----------------------------------------------------------------------------

// CancelCheck is the booking state cancellation is judged from.
type CancelCheck struct {
	Status string
	// EventStart is the event's first day. Cancellation closes at the START of
	// that day, so a buyer can cancel up to the night before but never on the
	// day the event runs.
	EventStart time.Time
	Total      float64
}

// CanCancel reports whether a booking may still be cancelled, with the reason
// when it may not.
func CanCancel(c CancelCheck, now time.Time) (bool, string) {
	switch c.Status {
	case StatusCancelled:
		return false, "this booking is already cancelled"
	case StatusExpired:
		return false, "this booking expired and was never paid"
	case StatusPendingPayment, StatusPaymentProcessing:
		return false, "this booking is not paid yet — it will release on its own if you do not pay"
	case StatusConfirmed:
		// fall through to the timing rule
	default:
		return false, "this booking cannot be cancelled"
	}

	if !EventNotStarted(c.EventStart, now) {
		return false, "the event has started — tickets can no longer be cancelled"
	}
	return true, ""
}

// EventNotStarted reports whether `now` is before the event's first day begins.
//
// The comparison is against midnight at the START of EventStart's day: an event
// dated today is already under way as far as cancellation is concerned, which
// is the conservative reading and avoids arguing about showtime.
func EventNotStarted(eventStart, now time.Time) bool {
	startOfDay := time.Date(
		eventStart.Year(), eventStart.Month(), eventStart.Day(),
		0, 0, 0, 0, eventStart.Location())
	return now.Before(startOfDay)
}

// ValidateCancelRequest checks the reason a family gave. A reason is mandatory:
// it is what finance reads when settling the refund by hand.
func ValidateCancelRequest(reason, note string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return httpx.ErrBadRequest("a cancellation reason is required")
	}
	if _, ok := CancellationReasons[reason]; !ok {
		return httpx.ErrBadRequest("unknown cancellation reason: " + reason)
	}
	// "Other" is only meaningful with an explanation attached.
	if reason == "other" && strings.TrimSpace(note) == "" {
		return httpx.ErrBadRequest(`please describe the reason when choosing "Other"`)
	}
	if len(note) > 500 {
		return httpx.ErrBadRequest("the cancellation note is too long (max 500 characters)")
	}
	return nil
}

// RefundPlan is what cancelling a booking owes the buyer.
type RefundPlan struct {
	Status string  `json:"status"`
	Amount float64 `json:"amount"`
	// Message is shown to the buyer at the moment they cancel, so the offline
	// process is never a surprise.
	Message string `json:"message"`
}

// PlanRefund decides what is owed. There is no online reversal anywhere in this
// system: a paid booking becomes a finance work item, and an unpaid one owes
// nothing at all.
func PlanRefund(status string, total float64) RefundPlan {
	if status != StatusConfirmed || total <= 0 {
		return RefundPlan{
			Status:  RefundNotApplicable,
			Amount:  0,
			Message: "No payment was taken, so there is nothing to refund.",
		}
	}
	return RefundPlan{
		Status: RefundPendingOffline,
		Amount: round2(total),
		Message: "Your refund has been logged and will be processed offline by our team. " +
			"Refunds are not returned automatically to your payment method.",
	}
}

// -----------------------------------------------------------------------------
// References
// -----------------------------------------------------------------------------

// refAlphabet omits I, O, 0 and 1 so a reference read aloud over the phone or
// copied off a screen cannot be transcribed wrongly.
const refAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// FormatReference renders a booking reference from n random bytes. The caller
// supplies the randomness so tests are deterministic and the rule stays pure.
func FormatReference(random []byte) string {
	var b strings.Builder
	b.WriteString("SX-")
	for _, r := range random {
		b.WriteByte(refAlphabet[int(r)%len(refAlphabet)])
	}
	return b.String()
}
