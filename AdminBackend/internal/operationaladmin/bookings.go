package operationaladmin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Ticket sales oversight — Operational Admin.
//
// Answers "how many tickets are booked for this event?" and gives Ops the one
// action the participant side deliberately cannot perform: marking an offline
// refund as settled. Money is never returned automatically anywhere in this
// system, so a cancelled booking sits in `pending_offline` until someone here
// says it has been paid out.
//
// This is read-mostly on purpose. Ops does not create or alter bookings —
// those belong to the family that made them — so there is no endpoint to do so.

// EventTicketStats is one line of the sales overview, read from the
// v_event_booking_stats view so the aggregation rules live in the schema.
type EventTicketStats struct {
	EventID   string    `json:"eventId"`
	EventName string    `json:"eventName"`
	StartDate time.Time `json:"startDate"`
	// EventStatus is the event's own lifecycle (open/live/completed), not a
	// booking status.
	EventStatus string `json:"eventStatus"`

	ConfirmedBookings  int `json:"confirmedBookings"`
	CancelledBookings  int `json:"cancelledBookings"`
	InProgressBookings int `json:"inProgressBookings"`

	// TicketsBooked counts seats currently sold; TicketsCancelled counts seats
	// that came back. They are reported separately rather than netted, because
	// Ops needs both numbers.
	TicketsBooked    int `json:"ticketsBooked"`
	TicketsCancelled int `json:"ticketsCancelled"`

	GrossRevenue    float64 `json:"grossRevenue"`
	RefundPending   float64 `json:"refundPending"`
	RefundCompleted float64 `json:"refundCompleted"`

	// Live seat state for the event, so Ops can see remaining capacity.
	SeatsHeld      int `json:"seatsHeld"`
	SeatsAvailable int `json:"seatsAvailable"`
	// MappedSeats is the SELLABLE count (blocked and house seats excluded), so
	// SeatsAvailable below is a number Ops can actually sell.
	MappedSeats int `json:"mappedSeats"`
	// Blocked and house seats are reported separately: they exist in the hall
	// and Ops must account for them, but they are never available.
	BlockedSeats int `json:"blockedSeats"`
	HouseSeats   int `json:"houseSeats"`
}

// BookingItemRow is one seat on an admin-visible booking.
type BookingItemRow struct {
	SeatLabel string  `json:"seatLabel"`
	Section   string  `json:"section"`
	RowLabel  string  `json:"rowLabel"`
	ZoneName  string  `json:"zoneName"`
	Price     float64 `json:"price"`
	Status    string  `json:"status"`
}

// BookingRow is one booking as Ops sees it, including who made it.
type BookingRow struct {
	ID         string    `json:"id"`
	Reference  string    `json:"reference"`
	EventID    string    `json:"eventId"`
	EventName  string    `json:"eventName"`
	FamilyName string    `json:"familyName"`
	Phone      string    `json:"phone"`
	Status     string    `json:"status"`
	SeatCount  int       `json:"seatCount"`
	Total      float64   `json:"total"`
	CouponCode string    `json:"couponCode,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`

	ConfirmedAt *time.Time `json:"confirmedAt,omitempty"`
	CancelledAt *time.Time `json:"cancelledAt,omitempty"`

	CancellationReason string `json:"cancellationReason,omitempty"`
	CancellationNote   string `json:"cancellationNote,omitempty"`
	CancelledBy        string `json:"cancelledBy,omitempty"`

	RefundStatus string  `json:"refundStatus"`
	RefundAmount float64 `json:"refundAmount"`
	RefundNote   string  `json:"refundNote,omitempty"`

	Items []BookingItemRow `json:"items"`
}

// refundUpdate settles (or un-settles) an offline refund.
type refundUpdate struct {
	// completed | pending_offline
	RefundStatus string `json:"refundStatus"`
	// Note is where the payout reference goes — a bank UTR, a cash receipt no.
	Note string `json:"note"`
}

type bookingService struct{ pool *pgxpool.Pool }

// stats returns the per-event sales overview. The booking aggregates come from
// the view; the live seat counts are joined on separately because they describe
// current state rather than history.
func (s bookingService) stats(ctx context.Context) ([]EventTicketStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT v.event_id::text, v.event_name, v.start_date, v.event_status,
		       v.confirmed_bookings, v.cancelled_bookings, v.in_progress_bookings,
		       v.tickets_booked, v.tickets_cancelled,
		       v.gross_revenue, v.refund_pending, v.refund_completed,
		       -- Seats currently held by a buyer mid-purchase. Judged by time,
		       -- exactly as the participant API judges it.
		       COALESCE((
		           SELECT count(*) FROM event_seat_status ess
		           WHERE ess.event_id = v.event_id AND ess.status = 'held'
		             AND ess.hold_expires_at > now()
		       ), 0),
		       -- The hall's sellable seat count, so "available" is meaningful.
		       COALESCE(m.total_seats, 0),
		       -- Seats Ops took out of service, and seats held back for the
		       -- house. Neither is sellable, so both sit outside total_seats.
		       COALESCE((
		           SELECT count(*) FROM admin_hall_seats hs
		           WHERE hs.seat_map_id = m.id AND NOT hs.is_active
		       ), 0),
		       COALESCE((
		           SELECT count(*) FROM admin_hall_seats hs
		           WHERE hs.seat_map_id = m.id AND hs.is_active AND hs.kind = 'house'
		       ), 0)
		FROM v_event_booking_stats v
		JOIN events e ON e.id = v.event_id
		LEFT JOIN admin_hall_seat_maps m ON m.hall_id = e.hall_id AND m.status = 'published'
		-- Only events that actually sell reserved seats belong on this screen.
		WHERE m.id IS NOT NULL
		ORDER BY v.start_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []EventTicketStats{}
	for rows.Next() {
		var r EventTicketStats
		if err := rows.Scan(&r.EventID, &r.EventName, &r.StartDate, &r.EventStatus,
			&r.ConfirmedBookings, &r.CancelledBookings, &r.InProgressBookings,
			&r.TicketsBooked, &r.TicketsCancelled,
			&r.GrossRevenue, &r.RefundPending, &r.RefundCompleted,
			&r.SeatsHeld, &r.MappedSeats, &r.BlockedSeats, &r.HouseSeats); err != nil {
			return nil, err
		}
		// Derived rather than queried: whatever is mapped and neither sold nor
		// held is still buyable.
		r.SeatsAvailable = r.MappedSeats - r.TicketsBooked - r.SeatsHeld
		if r.SeatsAvailable < 0 {
			r.SeatsAvailable = 0
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// list returns bookings, newest first, optionally filtered to one event or one
// status. Cancelled bookings are always included — the history is the point.
func (s bookingService) list(ctx context.Context, eventID, status string) ([]BookingRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text, b.reference, b.event_id::text, e.name,
		       COALESCE(f.display_name, ''), COALESCE(f.phone, ''),
		       b.status, b.seat_count, b.total, b.coupon_code, b.created_at,
		       b.confirmed_at, b.cancelled_at,
		       b.cancellation_reason, b.cancellation_note, b.cancelled_by,
		       b.refund_status, b.refund_amount, b.refund_note
		FROM bookings b
		JOIN events e ON e.id = b.event_id
		LEFT JOIN families f ON f.id = b.family_id
		WHERE ($1 = '' OR b.event_id = $1::uuid)
		  AND ($2 = '' OR b.status = $2)
		ORDER BY b.created_at DESC
		LIMIT 500`, eventID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BookingRow{}
	ids := []string{}
	for rows.Next() {
		var r BookingRow
		if err := rows.Scan(&r.ID, &r.Reference, &r.EventID, &r.EventName,
			&r.FamilyName, &r.Phone, &r.Status, &r.SeatCount, &r.Total,
			&r.CouponCode, &r.CreatedAt, &r.ConfirmedAt, &r.CancelledAt,
			&r.CancellationReason, &r.CancellationNote, &r.CancelledBy,
			&r.RefundStatus, &r.RefundAmount, &r.RefundNote); err != nil {
			return nil, err
		}
		r.Items = []BookingItemRow{}
		out = append(out, r)
		ids = append(ids, r.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return out, nil
	}

	// One query for every booking's seats rather than one per booking.
	itemRows, err := s.pool.Query(ctx, `
		SELECT booking_id::text, seat_label, section, row_label, zone_name, price, status
		FROM booking_items
		WHERE booking_id = ANY($1::uuid[])
		ORDER BY section, row_index, seat_number`, ids)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()

	byBooking := map[string][]BookingItemRow{}
	for itemRows.Next() {
		var bid string
		var it BookingItemRow
		if err := itemRows.Scan(&bid, &it.SeatLabel, &it.Section, &it.RowLabel,
			&it.ZoneName, &it.Price, &it.Status); err != nil {
			return nil, err
		}
		byBooking[bid] = append(byBooking[bid], it)
	}
	if err := itemRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if items, ok := byBooking[out[i].ID]; ok {
			out[i].Items = items
		}
	}
	return out, nil
}

// settleRefund records that an offline refund has been paid out (or puts it
// back on the queue). Only a cancelled booking with money owed can be settled —
// marking anything else "refunded" would be a bookkeeping fiction.
func (s bookingService) settleRefund(ctx context.Context, id string, u refundUpdate) (*BookingRow, error) {
	if u.RefundStatus != "completed" && u.RefundStatus != "pending_offline" {
		return nil, httpx.ErrBadRequest(`refundStatus must be "completed" or "pending_offline"`)
	}

	var status, refundStatus string
	if err := s.pool.QueryRow(ctx,
		`SELECT status, refund_status FROM bookings WHERE id=$1`, id).
		Scan(&status, &refundStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound("booking not found")
		}
		return nil, err
	}
	if status != "cancelled" {
		return nil, httpx.ErrConflict("only a cancelled booking can have a refund settled")
	}
	if refundStatus == "not_applicable" {
		return nil, httpx.ErrConflict("this booking has no refund owed")
	}

	var refundedAt any
	if u.RefundStatus == "completed" {
		refundedAt = time.Now()
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE bookings SET refund_status=$2, refund_note=$3, refunded_at=$4, updated_at=now()
		WHERE id=$1`, id, u.RefundStatus, u.Note, refundedAt); err != nil {
		return nil, err
	}

	list, err := s.list(ctx, "", "")
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, httpx.ErrNotFound("booking not found")
}

func registerBookings(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := bookingService{pool}

	// "How many tickets are booked?" — the overview screen.
	mux.Handle("GET /admin/ops/ticket-stats", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			data, err := svc.stats(r.Context())
			respond(w, data, err)
		})))

	// All bookings, optionally filtered: ?eventId=…&status=…
	mux.Handle("GET /admin/ops/bookings", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			data, err := svc.list(r.Context(),
				r.URL.Query().Get("eventId"), r.URL.Query().Get("status"))
			respond(w, data, err)
		})))

	// The bookings for one event.
	mux.Handle("GET /admin/ops/events/{id}/bookings", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			data, err := svc.list(r.Context(), r.PathValue("id"), r.URL.Query().Get("status"))
			respond(w, data, err)
		})))

	// Mark an offline refund as paid out.
	mux.Handle("PATCH /admin/ops/bookings/{id}/refund", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var u refundUpdate
			if err := httpx.Decode(r, &u); err != nil {
				httpx.Error(w, err)
				return
			}
			out, err := svc.settleRefund(r.Context(), r.PathValue("id"), u)
			respond(w, out, err)
		})))
}
