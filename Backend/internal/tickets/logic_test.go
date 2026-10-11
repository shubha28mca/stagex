package tickets

import (
	"errors"
	"testing"
	"time"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Unit tests for the booking rules: pricing, the seat-availability verdict that
// drives concurrency, hold expiry and the payment grace window, payment gating,
// the confirm decision table (including the money-taken-but-seats-lost edge
// case), cancellation eligibility and offline refunds.
//
// None of these need a database or a real clock — every rule takes `now` as an
// argument, which is what makes the time-sensitive behaviour testable at all.

// A fixed clock. All times in these tests are relative to it.
var now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

const grace = 5 * time.Minute
const ttl = 15 * time.Minute

func apiErr(t *testing.T, err error, wantStatus int) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var e *httpx.APIError
	if !errors.As(err, &e) {
		t.Fatalf("expected *httpx.APIError, got %T: %v", err, err)
	}
	if e.Status != wantStatus {
		t.Fatalf("status = %d, want %d (%s)", e.Status, wantStatus, e.Message)
	}
	return e.Message
}

// -----------------------------------------------------------------------------
// Pricing
// -----------------------------------------------------------------------------

func TestComputeTotals(t *testing.T) {
	seats := []SeatPrice{
		{SeatID: "1", SeatLabel: "A1", Price: 2500},
		{SeatID: "2", SeatLabel: "A2", Price: 2500},
		{SeatID: "3", SeatLabel: "P5", Price: 450},
	}
	got := ComputeTotals(seats, 500)
	if got.SeatCount != 3 {
		t.Errorf("SeatCount = %d, want 3", got.SeatCount)
	}
	if got.Subtotal != 5450 {
		t.Errorf("Subtotal = %v, want 5450", got.Subtotal)
	}
	if got.Discount != 500 {
		t.Errorf("Discount = %v, want 500", got.Discount)
	}
	if got.Total != 4950 {
		t.Errorf("Total = %v, want 4950", got.Total)
	}
}

func TestComputeTotalsEmptySelection(t *testing.T) {
	got := ComputeTotals(nil, 100)
	// With nothing selected there is nothing to discount either.
	if got.Subtotal != 0 || got.Total != 0 || got.Discount != 0 || got.SeatCount != 0 {
		t.Errorf("empty selection produced %+v, want all zeroes", got)
	}
}

func TestComputeTotalsClampsDiscountToSubtotal(t *testing.T) {
	// An over-generous coupon must never make the total negative — that would
	// read as the platform owing the buyer money.
	got := ComputeTotals([]SeatPrice{{Price: 300}}, 1000)
	if got.Discount != 300 {
		t.Errorf("Discount = %v, want it clamped to 300", got.Discount)
	}
	if got.Total != 0 {
		t.Errorf("Total = %v, want 0 — never negative", got.Total)
	}
}

func TestComputeTotalsIgnoresNegativeDiscount(t *testing.T) {
	got := ComputeTotals([]SeatPrice{{Price: 300}}, -50)
	if got.Discount != 0 || got.Total != 300 {
		t.Errorf("got %+v, want discount 0 and total 300", got)
	}
}

func TestComputeTotalsRoundsToPaise(t *testing.T) {
	// Ten seats at 33.33 accumulate float error; the stored total must match
	// the sum of the printed tickets.
	seats := make([]SeatPrice, 10)
	for i := range seats {
		seats[i] = SeatPrice{Price: 33.33}
	}
	got := ComputeTotals(seats, 0)
	if got.Subtotal != 333.30 {
		t.Errorf("Subtotal = %v, want 333.30", got.Subtotal)
	}
	if got.Total != 333.30 {
		t.Errorf("Total = %v, want 333.30", got.Total)
	}
}

func TestComputeTotalsFreeSeats(t *testing.T) {
	// A zero-priced seat is legitimate (a comp), and must not be mistaken for
	// an unpriced one at this layer.
	got := ComputeTotals([]SeatPrice{{Price: 0}, {Price: 0}}, 0)
	if got.SeatCount != 2 || got.Total != 0 {
		t.Errorf("got %+v, want 2 seats totalling 0", got)
	}
}

// -----------------------------------------------------------------------------
// Seat selection validation
// -----------------------------------------------------------------------------

func TestValidateSeatSelection(t *testing.T) {
	if err := ValidateSeatSelection([]string{"a", "b"}); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}

	apiErr(t, ValidateSeatSelection(nil), 400)
	apiErr(t, ValidateSeatSelection([]string{}), 400)
	apiErr(t, ValidateSeatSelection([]string{"a", "a"}), 400)
	apiErr(t, ValidateSeatSelection([]string{"a", "  "}), 400)

	// Exactly at the cap is fine; one more is not.
	atCap := make([]string, MaxSeatsPerBooking)
	for i := range atCap {
		atCap[i] = string(rune('a' + i))
	}
	if err := ValidateSeatSelection(atCap); err != nil {
		t.Errorf("%d seats should be allowed: %v", MaxSeatsPerBooking, err)
	}
	apiErr(t, ValidateSeatSelection(append(atCap, "over")), 400)
}

// -----------------------------------------------------------------------------
// Availability — the read side of the concurrency model
// -----------------------------------------------------------------------------

func TestAvailabilityNoRowMeansAvailable(t *testing.T) {
	// A seat nobody has ever touched has no row in event_seat_status.
	if got := Availability(SeatState{}, "fam-1", now, grace); got != AvailAvailable {
		t.Errorf("zero state = %q, want %q", got, AvailAvailable)
	}
}

func TestAvailabilityBookedIsFinal(t *testing.T) {
	st := SeatState{Status: SeatBooked, BookingID: "b1"}
	if got := Availability(st, "fam-1", now, grace); got != AvailBooked {
		t.Errorf("got %q, want %q", got, AvailBooked)
	}
	// Even the buyer sees "booked" rather than "yours to hold".
	if CanClaim(st, "fam-1", now, grace) {
		t.Error("a booked seat must never be claimable")
	}
}

func TestAvailabilityDistinguishesYourOwnHold(t *testing.T) {
	st := SeatState{
		Status: SeatHeld, HeldBy: "fam-1", HoldGroupID: "g1",
		HoldExpiresAt: now.Add(10 * time.Minute),
	}
	// This is the behaviour the requirement calls for: the moment one user
	// claims a seat, everyone else sees it as taken.
	if got := Availability(st, "fam-1", now, grace); got != AvailHeldByYou {
		t.Errorf("holder sees %q, want %q", got, AvailHeldByYou)
	}
	if got := Availability(st, "fam-2", now, grace); got != AvailHeld {
		t.Errorf("other user sees %q, want %q", got, AvailHeld)
	}
	// The holder may re-claim (that is how extending a selection works); the
	// other user may not.
	if !CanClaim(st, "fam-1", now, grace) {
		t.Error("the holder must be able to re-claim their own seat")
	}
	if CanClaim(st, "fam-2", now, grace) {
		t.Error("another family must not be able to claim a held seat")
	}
}

func TestAvailabilityExpiredHoldReturnsToAvailable(t *testing.T) {
	// The 15-minute rule: once the window passes with no payment, the seat is
	// free again — and it becomes free by TIME, with no sweeper involved.
	st := SeatState{
		Status: SeatHeld, HeldBy: "fam-1",
		HoldExpiresAt: now.Add(-1 * time.Second),
	}
	if got := Availability(st, "fam-2", now, grace); got != AvailAvailable {
		t.Errorf("lapsed hold = %q, want %q", got, AvailAvailable)
	}
	if !CanClaim(st, "fam-2", now, grace) {
		t.Error("another family must be able to take a seat whose hold lapsed")
	}
}

func TestAvailabilityHoldIsExclusiveRightUpToTheDeadline(t *testing.T) {
	st := SeatState{Status: SeatHeld, HeldBy: "fam-1", HoldExpiresAt: now.Add(ttl)}

	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"just taken", now, AvailHeld},
		{"one second before expiry", now.Add(ttl - time.Second), AvailHeld},
		{"exactly at expiry", now.Add(ttl), AvailAvailable},
		{"after expiry", now.Add(ttl + time.Second), AvailAvailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Availability(st, "fam-2", c.at, grace); got != c.want {
				t.Errorf("at %v: got %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestAvailabilityPaymentInFlightSurvivesTheDeadline(t *testing.T) {
	// The edge case: the buyer's 15 minutes ran out while the gateway was still
	// authorising. Releasing the seat here would let someone else buy it and
	// leave us double-sold, so the hold stretches by the grace window.
	st := SeatState{
		Status: SeatHeld, HeldBy: "fam-1",
		HoldExpiresAt:   now,
		PaymentInFlight: true,
	}
	if got := Availability(st, "fam-2", now.Add(time.Minute), grace); got != AvailHeld {
		t.Errorf("during grace: got %q, want %q", got, AvailHeld)
	}
	if got := Availability(st, "fam-2", now.Add(grace-time.Second), grace); got != AvailHeld {
		t.Errorf("late in grace: got %q, want %q", got, AvailHeld)
	}
	// Grace is finite: a payment that never resolves cannot park the seat forever.
	if got := Availability(st, "fam-2", now.Add(grace), grace); got != AvailAvailable {
		t.Errorf("after grace: got %q, want %q", got, AvailAvailable)
	}

	// Without a payment in flight the same expiry frees the seat immediately.
	noPayment := st
	noPayment.PaymentInFlight = false
	if got := Availability(noPayment, "fam-2", now.Add(time.Minute), grace); got != AvailAvailable {
		t.Errorf("no payment in flight: got %q, want %q", got, AvailAvailable)
	}
}

func TestAvailabilityExplicitlyReleasedSeat(t *testing.T) {
	// A released hold blanks the status but may leave the timestamp behind; the
	// status check must not be fooled by a future expiry.
	st := SeatState{Status: SeatAvailable, HoldExpiresAt: now.Add(time.Hour)}
	if got := Availability(st, "fam-2", now, grace); got != AvailAvailable {
		t.Errorf("got %q, want %q", got, AvailAvailable)
	}
}

func TestHoldActiveAndDeadlines(t *testing.T) {
	if got := HoldDeadline(now, ttl); !got.Equal(now.Add(ttl)) {
		t.Errorf("HoldDeadline = %v, want %v", got, now.Add(ttl))
	}
	if got := EffectiveDeadline(now, false, grace); !got.Equal(now) {
		t.Errorf("EffectiveDeadline without payment = %v, want %v", got, now)
	}
	if got := EffectiveDeadline(now, true, grace); !got.Equal(now.Add(grace)) {
		t.Errorf("EffectiveDeadline with payment = %v, want %v", got, now.Add(grace))
	}
	// A seat that is available or booked is never "actively held".
	for _, s := range []string{SeatAvailable, SeatBooked} {
		if HoldActive(SeatState{Status: s, HoldExpiresAt: now.Add(time.Hour)}, now, grace) {
			t.Errorf("status %q must not count as an active hold", s)
		}
	}
}

// -----------------------------------------------------------------------------
// Conflict reporting
// -----------------------------------------------------------------------------

func TestConflictError(t *testing.T) {
	if ConflictError(nil) != nil {
		t.Error("no conflicts must produce no error")
	}

	msg := apiErr(t, ConflictError([]SeatConflict{{SeatLabel: "A12", Reason: "held"}}), 409)
	if !contains(msg, "A12") {
		t.Errorf("single-seat message %q should name the seat", msg)
	}

	msg = apiErr(t, ConflictError([]SeatConflict{
		{SeatLabel: "A12"}, {SeatLabel: "A13"},
	}), 409)
	if !contains(msg, "A12") || !contains(msg, "A13") {
		t.Errorf("multi-seat message %q should name every seat", msg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// -----------------------------------------------------------------------------
// Payment gating
// -----------------------------------------------------------------------------

func TestCanStartPayment(t *testing.T) {
	valid := PayabilityInput{Status: StatusPendingPayment, HoldExpiresAt: now.Add(ttl)}
	if ok, why := CanStartPayment(valid, now); !ok {
		t.Fatalf("a fresh pending booking must be payable: %s", why)
	}

	cases := []struct {
		name string
		in   PayabilityInput
	}{
		{"already confirmed", PayabilityInput{Status: StatusConfirmed, HoldExpiresAt: now.Add(ttl)}},
		{"cancelled", PayabilityInput{Status: StatusCancelled, HoldExpiresAt: now.Add(ttl)}},
		{"expired", PayabilityInput{Status: StatusExpired, HoldExpiresAt: now.Add(ttl)}},
		{"already processing", PayabilityInput{Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(ttl)}},
		{"hold lapsed", PayabilityInput{Status: StatusPendingPayment, HoldExpiresAt: now.Add(-time.Second)}},
		{"out of attempts", PayabilityInput{Status: StatusPendingPayment, HoldExpiresAt: now.Add(ttl), Attempts: MaxPaymentAttempts}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, why := CanStartPayment(c.in, now)
			if ok {
				t.Error("expected payment to be refused")
			}
			if why == "" {
				t.Error("a refusal must explain itself")
			}
		})
	}

	// The last attempt is still allowed.
	last := PayabilityInput{
		Status: StatusPendingPayment, HoldExpiresAt: now.Add(ttl),
		Attempts: MaxPaymentAttempts - 1,
	}
	if ok, why := CanStartPayment(last, now); !ok {
		t.Errorf("attempt %d of %d must be allowed: %s", MaxPaymentAttempts, MaxPaymentAttempts, why)
	}
}

// -----------------------------------------------------------------------------
// The confirm decision table
// -----------------------------------------------------------------------------

func TestDecideConfirmHappyPath(t *testing.T) {
	out := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(5 * time.Minute),
		PaymentSucceeded: true, SeatsStillHeld: true,
	}, now, grace)
	if !out.Confirm {
		t.Fatalf("expected Confirm, got %+v", out)
	}
	if out.CancelWithRefund || out.Expire || out.Retry || out.AlreadyDone {
		t.Errorf("only Confirm should be set: %+v", out)
	}
}

func TestDecideConfirmIsIdempotent(t *testing.T) {
	// A gateway that delivers the same webhook twice, or a buyer who
	// double-clicks, must not book the seats twice or produce an error.
	for _, status := range []string{StatusConfirmed, StatusCancelled} {
		out := DecideConfirm(ConfirmInput{
			Status: status, PaymentSucceeded: true, SeatsStillHeld: true,
			HoldExpiresAt: now.Add(ttl),
		}, now, grace)
		if !out.AlreadyDone {
			t.Errorf("status %q: expected AlreadyDone, got %+v", status, out)
		}
		if out.Confirm {
			t.Errorf("status %q must not be confirmed again", status)
		}
	}
}

func TestDecideConfirmMoneyTakenButSeatsLost(t *testing.T) {
	// The worst case, and the reason confirmation re-checks the seats: the
	// charge succeeded but the seats had already gone to someone else.
	// Confirming would double-sell, so the only honest outcome is to cancel
	// and owe a refund.
	out := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(-10 * time.Minute),
		PaymentSucceeded: true, SeatsStillHeld: false,
	}, now, grace)
	if !out.CancelWithRefund {
		t.Fatalf("expected CancelWithRefund, got %+v", out)
	}
	if out.Confirm {
		t.Error("must never confirm a booking whose seats were lost")
	}
	if out.Reason == "" {
		t.Error("the buyer must be told what happened")
	}
}

func TestDecideConfirmSucceedsInsideGraceWindow(t *testing.T) {
	// A slow gateway that lands just after the 15 minutes still wins, provided
	// the seats were never released.
	out := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(-time.Minute),
		PaymentSucceeded: true, SeatsStillHeld: true,
	}, now, grace)
	if !out.Confirm {
		t.Fatalf("a payment inside the grace window must confirm, got %+v", out)
	}
}

func TestDecideConfirmSucceedsAfterGraceIfSeatsSurvived(t *testing.T) {
	// Beyond the grace window but the seats are demonstrably still ours —
	// honour the sale rather than punish the buyer for our gateway.
	out := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(-time.Hour),
		PaymentSucceeded: true, SeatsStillHeld: true,
	}, now, grace)
	if !out.Confirm {
		t.Fatalf("expected Confirm, got %+v", out)
	}
	if out.Reason == "" {
		t.Error("this unusual path should record why it was taken")
	}
}

func TestDecideConfirmFailureAllowsRetryInsideWindow(t *testing.T) {
	out := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(10 * time.Minute),
		PaymentSucceeded: false, SeatsStillHeld: true, Attempts: 1,
	}, now, grace)
	if !out.Retry {
		t.Fatalf("expected Retry, got %+v", out)
	}
	if out.Expire {
		t.Error("the seats must stay held so the buyer can try again")
	}
}

func TestDecideConfirmFailureExpiresWhenOutOfTimeOrAttempts(t *testing.T) {
	outOfTime := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(-grace - time.Minute),
		PaymentSucceeded: false, SeatsStillHeld: true, Attempts: 1,
	}, now, grace)
	if !outOfTime.Expire {
		t.Errorf("a failure past the window must expire the hold, got %+v", outOfTime)
	}

	outOfAttempts := DecideConfirm(ConfirmInput{
		Status: StatusPaymentProcessing, HoldExpiresAt: now.Add(10 * time.Minute),
		PaymentSucceeded: false, SeatsStillHeld: true, Attempts: MaxPaymentAttempts,
	}, now, grace)
	if !outOfAttempts.Expire {
		t.Errorf("the final failure must release the seats, got %+v", outOfAttempts)
	}
	if outOfAttempts.Retry {
		t.Error("no retry once the attempt limit is reached")
	}
}

// -----------------------------------------------------------------------------
// Cancellation
// -----------------------------------------------------------------------------

func TestCanCancelConfirmedBookingBeforeTheEvent(t *testing.T) {
	ok, why := CanCancel(CancelCheck{
		Status: StatusConfirmed, EventStart: now.Add(72 * time.Hour), Total: 1000,
	}, now)
	if !ok {
		t.Fatalf("a paid booking three days out must be cancellable: %s", why)
	}
}

func TestCanCancelRefusedOnceTheEventStarts(t *testing.T) {
	// The requirement: once the event starts, cancellation is closed.
	started := CancelCheck{Status: StatusConfirmed, EventStart: now.Add(-time.Hour), Total: 1000}
	ok, why := CanCancel(started, now)
	if ok {
		t.Fatal("a started event must not be cancellable")
	}
	if !contains(why, "started") {
		t.Errorf("reason %q should say the event has started", why)
	}

	// An event dated today counts as under way — the conservative reading, so
	// nobody argues about showtime.
	today := CancelCheck{Status: StatusConfirmed, EventStart: now, Total: 1000}
	if ok, _ := CanCancel(today, now); ok {
		t.Error("an event dated today must not be cancellable")
	}
}

func TestCanCancelAllowsRightUpToMidnightBefore(t *testing.T) {
	eventDay := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	c := CancelCheck{Status: StatusConfirmed, EventStart: eventDay, Total: 500}

	justBefore := eventDay.Add(-time.Second)
	if ok, why := CanCancel(c, justBefore); !ok {
		t.Errorf("one second before the event day must still allow cancelling: %s", why)
	}
	if ok, _ := CanCancel(c, eventDay); ok {
		t.Error("at midnight on the event day cancelling must be closed")
	}
}

func TestCanCancelRejectsNonCancellableStates(t *testing.T) {
	future := now.Add(72 * time.Hour)
	cases := []string{
		StatusCancelled, StatusExpired, StatusPendingPayment, StatusPaymentProcessing,
	}
	for _, status := range cases {
		t.Run(status, func(t *testing.T) {
			ok, why := CanCancel(CancelCheck{Status: status, EventStart: future}, now)
			if ok {
				t.Errorf("status %q must not be cancellable", status)
			}
			if why == "" {
				t.Error("a refusal must explain itself")
			}
		})
	}
}

func TestEventNotStarted(t *testing.T) {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	// A DATE column arrives as midnight; anything on that day is "started".
	if !EventNotStarted(day, day.Add(-time.Minute)) {
		t.Error("the minute before the event day should count as not started")
	}
	if EventNotStarted(day, day) {
		t.Error("midnight on the event day should count as started")
	}
	if EventNotStarted(day, day.Add(23*time.Hour)) {
		t.Error("late on the event day should count as started")
	}
	// Even an event stored with a time-of-day is judged from the start of its day.
	withTime := time.Date(2026, 3, 15, 19, 30, 0, 0, time.UTC)
	if EventNotStarted(withTime, time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)) {
		t.Error("the morning of the event day should count as started")
	}
}

func TestValidateCancelRequest(t *testing.T) {
	if err := ValidateCancelRequest("change_of_plan", ""); err != nil {
		t.Fatalf("a known reason with no note must be accepted: %v", err)
	}
	if err := ValidateCancelRequest("other", "moving house that weekend"); err != nil {
		t.Fatalf("'other' with a note must be accepted: %v", err)
	}

	// A reason is mandatory — it is what finance reads when settling by hand.
	apiErr(t, ValidateCancelRequest("", "note"), 400)
	apiErr(t, ValidateCancelRequest("   ", "note"), 400)
	apiErr(t, ValidateCancelRequest("because", ""), 400)
	// "Other" without an explanation carries no information.
	apiErr(t, ValidateCancelRequest("other", "   "), 400)

	long := make([]byte, 501)
	for i := range long {
		long[i] = 'x'
	}
	apiErr(t, ValidateCancelRequest("change_of_plan", string(long)), 400)
}

func TestCancellationReasonsAreStable(t *testing.T) {
	// The frontend renders this set; every entry needs a label to show.
	if len(CancellationReasons) < 3 {
		t.Fatal("expected a usable set of cancellation reasons")
	}
	if _, ok := CancellationReasons["other"]; !ok {
		t.Error(`"other" must always be offered as an escape hatch`)
	}
	for code, label := range CancellationReasons {
		if code == "" || label == "" {
			t.Errorf("reason %q -> %q: neither may be empty", code, label)
		}
	}
}

// -----------------------------------------------------------------------------
// Refunds — offline only
// -----------------------------------------------------------------------------

func TestPlanRefundForAPaidBooking(t *testing.T) {
	p := PlanRefund(StatusConfirmed, 4950)
	if p.Status != RefundPendingOffline {
		t.Errorf("Status = %q, want %q", p.Status, RefundPendingOffline)
	}
	if p.Amount != 4950 {
		t.Errorf("Amount = %v, want 4950", p.Amount)
	}
	// The buyer must be told up front that this is not an automatic reversal.
	if !contains(p.Message, "offline") {
		t.Errorf("message %q should say the refund is handled offline", p.Message)
	}
}

func TestPlanRefundOwesNothingWhenNoMoneyWasTaken(t *testing.T) {
	for _, status := range []string{StatusPendingPayment, StatusExpired, StatusCancelled} {
		p := PlanRefund(status, 1000)
		if p.Status != RefundNotApplicable {
			t.Errorf("status %q: refund %q, want %q", status, p.Status, RefundNotApplicable)
		}
		if p.Amount != 0 {
			t.Errorf("status %q: amount %v, want 0", status, p.Amount)
		}
	}
	// A confirmed but free booking owes nothing either.
	if p := PlanRefund(StatusConfirmed, 0); p.Status != RefundNotApplicable {
		t.Errorf("a zero-value booking should owe nothing, got %q", p.Status)
	}
}

func TestPlanRefundRoundsTheAmount(t *testing.T) {
	if p := PlanRefund(StatusConfirmed, 333.333); p.Amount != 333.33 {
		t.Errorf("Amount = %v, want 333.33", p.Amount)
	}
}

// -----------------------------------------------------------------------------
// References
// -----------------------------------------------------------------------------

func TestFormatReference(t *testing.T) {
	ref := FormatReference([]byte{0, 1, 2, 3, 4, 5})
	if len(ref) != 3+6 {
		t.Errorf("reference %q has length %d, want 9", ref, len(ref))
	}
	if ref[:3] != "SX-" {
		t.Errorf("reference %q should start with SX-", ref)
	}
	// Deterministic for a given input, which is what makes it testable.
	if again := FormatReference([]byte{0, 1, 2, 3, 4, 5}); again != ref {
		t.Errorf("not deterministic: %q vs %q", ref, again)
	}
}

func TestFormatReferenceAvoidsAmbiguousCharacters(t *testing.T) {
	// I, O, 0 and 1 are excluded so a reference read out over the phone cannot
	// be transcribed wrongly. Sweep the whole byte range to prove it.
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	ref := FormatReference(all)
	for _, bad := range []rune{'I', 'O', '0', '1'} {
		for _, c := range ref[3:] {
			if c == bad {
				t.Errorf("reference contains the ambiguous character %q", bad)
				break
			}
		}
	}
}
