package tickets

import "time"

// Wire shapes for the booking flow. Read models first, then request bodies.

// -----------------------------------------------------------------------------
// The seat picker
// -----------------------------------------------------------------------------

// Seat is one seat as the buyer sees it: where it is, what it costs, and
// whether it can be taken. Availability is resolved server-side by the same
// rule the claim transaction enforces, so the picker never shows a lie.
type Seat struct {
	SeatID     string  `json:"seatId"`
	Section    string  `json:"section"`
	RowLabel   string  `json:"rowLabel"`
	RowIndex   int     `json:"rowIndex"`
	SeatNumber int     `json:"seatNumber"`
	SeatLabel  string  `json:"seatLabel"`
	ZoneCode   string  `json:"zoneCode"`
	ZoneName   string  `json:"zoneName"`
	ZoneColor  string  `json:"zoneColor"`
	Price      float64 `json:"price"`
	Kind       string  `json:"kind"`
	PosX       int     `json:"posX"`
	PosY       int     `json:"posY"`
	// available | held_by_you | held | booked
	Availability string `json:"availability"`
}

// Element is a non-seat item drawn on the plan (stage, backstage, aisle) so the
// buyer can orient themselves relative to the stage.
type Element struct {
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	PosX     int    `json:"posX"`
	PosY     int    `json:"posY"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Rotation int    `json:"rotation"`
	Color    string `json:"color"`
}

// Zone is a price tier, used for the picker's legend.
type Zone struct {
	Code  string  `json:"code"`
	Name  string  `json:"name"`
	Color string  `json:"color"`
	Price float64 `json:"price"`
}

// EventSeatMap is everything the seat picker needs in one response.
type EventSeatMap struct {
	EventID      string    `json:"eventId"`
	EventName    string    `json:"eventName"`
	EventStart   time.Time `json:"eventStart"`
	EventStatus  string    `json:"eventStatus"`
	HallName     string    `json:"hallName"`
	HallCity     string    `json:"hallCity"`
	CanvasWidth  int       `json:"canvasWidth"`
	CanvasHeight int       `json:"canvasHeight"`
	Zones        []Zone    `json:"zones"`
	Seats        []Seat    `json:"seats"`
	Elements     []Element `json:"elements"`

	AvailableCount int `json:"availableCount"`
	HeldCount      int `json:"heldCount"`
	BookedCount    int `json:"bookedCount"`

	// MaxSeatsPerBooking lets the picker enforce the same cap as the API.
	MaxSeatsPerBooking int `json:"maxSeatsPerBooking"`
	// HoldMinutes is the configured window, so the countdown the buyer sees and
	// the deadline the server enforces come from one place.
	HoldMinutes int `json:"holdMinutes"`

	// The caller's own live hold on this event, if any.
	HoldGroupID   string     `json:"holdGroupId,omitempty"`
	HoldExpiresAt *time.Time `json:"holdExpiresAt,omitempty"`
}

// -----------------------------------------------------------------------------
// Holds
// -----------------------------------------------------------------------------

// Hold is a successful claim: which seats, what they cost, and the deadline.
type Hold struct {
	HoldGroupID string      `json:"holdGroupId"`
	EventID     string      `json:"eventId"`
	ExpiresAt   time.Time   `json:"expiresAt"`
	Seats       []SeatPrice `json:"seats"`
	Totals      Totals      `json:"totals"`
}

type holdRequest struct {
	EventID string   `json:"eventId"`
	SeatIDs []string `json:"seatIds"`
}

// -----------------------------------------------------------------------------
// Bookings
// -----------------------------------------------------------------------------

// BookingItem is one seat on a booking. Every field is the value captured at
// purchase time, so a past ticket still reads correctly after Ops redesigns
// the hall.
type BookingItem struct {
	ID         string  `json:"id"`
	SeatID     string  `json:"seatId,omitempty"`
	Section    string  `json:"section"`
	RowLabel   string  `json:"rowLabel"`
	RowIndex   int     `json:"rowIndex"`
	SeatNumber int     `json:"seatNumber"`
	SeatLabel  string  `json:"seatLabel"`
	ZoneCode   string  `json:"zoneCode"`
	ZoneName   string  `json:"zoneName"`
	Price      float64 `json:"price"`
	Status     string  `json:"status"`
}

// Booking is one purchase and its whole history — including, once cancelled,
// why it was cancelled and what is owed. Bookings are never deleted.
type Booking struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	EventID   string `json:"eventId"`
	EventName string `json:"eventName"`
	EventCity string `json:"eventCity"`
	// EventStart drives the cancellation window, so the client can grey the
	// button out for the same reason the server would refuse.
	EventStart time.Time `json:"eventStart"`
	HallName   string    `json:"hallName"`

	Status     string  `json:"status"`
	SeatCount  int     `json:"seatCount"`
	Subtotal   float64 `json:"subtotal"`
	Discount   float64 `json:"discount"`
	Total      float64 `json:"total"`
	CouponCode string  `json:"couponCode,omitempty"`

	HoldExpiresAt time.Time  `json:"holdExpiresAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	ConfirmedAt   *time.Time `json:"confirmedAt,omitempty"`
	CancelledAt   *time.Time `json:"cancelledAt,omitempty"`

	CancellationReason string `json:"cancellationReason,omitempty"`
	// The human label for the reason code, so the client needs no lookup table.
	CancellationReasonLabel string `json:"cancellationReasonLabel,omitempty"`
	CancellationNote        string `json:"cancellationNote,omitempty"`
	CancelledBy             string `json:"cancelledBy,omitempty"`

	RefundStatus string  `json:"refundStatus"`
	RefundAmount float64 `json:"refundAmount"`
	RefundNote   string  `json:"refundNote,omitempty"`

	Items []BookingItem `json:"items,omitempty"`

	// Resolved server-side so the two never disagree.
	CanCancel       bool   `json:"canCancel"`
	CannotCancelWhy string `json:"cannotCancelWhy,omitempty"`
	// Attempts made so far, so the client can show "1 of 3".
	PaymentAttempts int `json:"paymentAttempts"`
}

type createBookingRequest struct {
	HoldGroupID string `json:"holdGroupId"`
	CouponCode  string `json:"couponCode"`
}

// PaymentIntent is the mock gateway's order, mirroring the registration
// payment flow so the frontend renders the same sheet.
type PaymentIntent struct {
	BookingID   string    `json:"bookingId"`
	ProviderRef string    `json:"providerRef"`
	Provider    string    `json:"provider"`
	Amount      float64   `json:"amount"`
	Attempt     int       `json:"attempt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type confirmRequest struct {
	ProviderRef string `json:"providerRef"`
	// Success simulates the gateway outcome. A real integration would verify a
	// signed webhook instead of trusting the client.
	Success bool `json:"success"`
}

// ConfirmResult reports what confirmation did. Outcome is one of:
// confirmed | already_confirmed | retry | expired | cancelled_refund_due.
type ConfirmResult struct {
	Outcome      string      `json:"outcome"`
	Message      string      `json:"message"`
	AttemptsLeft int         `json:"attemptsLeft"`
	Booking      *Booking    `json:"booking,omitempty"`
	Refund       *RefundPlan `json:"refund,omitempty"`
}

type cancelRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// CancelResult is what the buyer is told after cancelling.
type CancelResult struct {
	Booking *Booking   `json:"booking"`
	Refund  RefundPlan `json:"refund"`
}

// ReasonOption is one selectable cancellation reason for the client's dropdown.
type ReasonOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}
