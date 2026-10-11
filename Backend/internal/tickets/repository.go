package tickets

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Repository holds every SQL statement for booking. The transactional methods
// here are the ones that matter: HoldSeats and ApplyConfirm are where two
// buyers racing for the same seat is resolved.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds a booking repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// -----------------------------------------------------------------------------
// Seat picker read
// -----------------------------------------------------------------------------

// EventHeader is the event/hall context for a seat map.
type EventHeader struct {
	EventID      string
	EventName    string
	EventCity    string
	EventStart   time.Time
	EventStatus  string
	HallName     string
	HallCity     string
	CanvasWidth  int
	CanvasHeight int
}

// eventHeader loads the event and the published arrangement of its hall. It
// separates "no such event" from "this event has no reserved seating" so the
// UI can say something useful in each case.
func (r *Repository) eventHeader(ctx context.Context, eventID string) (*EventHeader, error) {
	var h EventHeader
	err := r.pool.QueryRow(ctx, `
		SELECT e.id, e.name, e.city, e.start_date, e.status,
		       h.name, h.city, m.canvas_width, m.canvas_height
		FROM events e
		JOIN admin_halls h ON h.id = e.hall_id
		JOIN admin_hall_seat_maps m ON m.hall_id = h.id AND m.status = 'published'
		WHERE e.id = $1`, eventID).
		Scan(&h.EventID, &h.EventName, &h.EventCity, &h.EventStart, &h.EventStatus,
			&h.HallName, &h.HallCity, &h.CanvasWidth, &h.CanvasHeight)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if e := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM events WHERE id=$1)`, eventID).Scan(&exists); e != nil {
			return nil, e
		}
		if !exists {
			return nil, httpx.ErrNotFound("event not found")
		}
		return nil, httpx.ErrNotFound(
			"this event does not have reserved seating — its hall has no published seating arrangement")
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// SeatMap returns the event's seats with each one's availability resolved for
// this viewer.
//
// Availability is computed in Go by Availability() rather than in SQL, so the
// read and the claim transaction share ONE implementation of the rule. The
// query's job is only to surface the raw state, including whether a hold
// belongs to a booking that is mid-payment (the grace-window case).
func (r *Repository) SeatMap(ctx context.Context, eventID, familyID string,
	now time.Time, grace time.Duration) (*EventSeatMap, error) {

	h, err := r.eventHeader(ctx, eventID)
	if err != nil {
		return nil, err
	}

	out := &EventSeatMap{
		EventID: h.EventID, EventName: h.EventName, EventStart: h.EventStart,
		EventStatus: h.EventStatus, HallName: h.HallName, HallCity: h.HallCity,
		CanvasWidth: h.CanvasWidth, CanvasHeight: h.CanvasHeight,
		MaxSeatsPerBooking: MaxSeatsPerBooking,
		Zones:              []Zone{}, Seats: []Seat{}, Elements: []Element{},
	}

	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.section, s.row_label, s.row_index, s.seat_number, s.seat_label,
		       COALESCE(z.code, ''), COALESCE(z.name, ''), COALESCE(z.color, '#6a1b9a'),
		       COALESCE(s.price, z.price, 0), s.kind, s.pos_x, s.pos_y,
		       COALESCE(ess.status, 'available'),
		       COALESCE(ess.held_by::text, ''),
		       COALESCE(ess.hold_group_id::text, ''),
		       ess.hold_expires_at,
		       -- Does this hold belong to a payment currently in flight? If so it
		       -- must survive past its deadline for the grace window.
		       COALESCE((
		           SELECT true FROM bookings b
		           WHERE b.hold_group_id = ess.hold_group_id
		             AND b.status = 'payment_processing'
		           LIMIT 1
		       ), false)
		FROM admin_hall_seats s
		JOIN admin_hall_seat_maps m ON m.id = s.seat_map_id AND m.status = 'published'
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id
		LEFT JOIN event_seat_status ess ON ess.event_id = e.id AND ess.seat_id = s.id
		-- Blocked seats and house holds are not sellable, so they are simply not
		-- offered to the buyer.
		WHERE s.is_active AND s.kind <> 'house'
		ORDER BY s.section, s.row_index, s.seat_number`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s Seat
		var st SeatState
		var expires *time.Time
		if err := rows.Scan(&s.SeatID, &s.Section, &s.RowLabel, &s.RowIndex, &s.SeatNumber,
			&s.SeatLabel, &s.ZoneCode, &s.ZoneName, &s.ZoneColor, &s.Price, &s.Kind,
			&s.PosX, &s.PosY, &st.Status, &st.HeldBy, &st.HoldGroupID, &expires,
			&st.PaymentInFlight); err != nil {
			return nil, err
		}
		if expires != nil {
			st.HoldExpiresAt = *expires
		}
		s.Availability = Availability(st, familyID, now, grace)

		switch s.Availability {
		case AvailBooked:
			out.BookedCount++
		case AvailHeld:
			out.HeldCount++
		default:
			// The viewer's own hold still counts as available to them: they can
			// keep it, so it is not lost capacity from their point of view.
			out.AvailableCount++
		}
		// Surface the caller's own live hold so a reload resumes the countdown.
		if s.Availability == AvailHeldByYou && out.HoldGroupID == "" {
			out.HoldGroupID = st.HoldGroupID
			exp := st.HoldExpiresAt
			out.HoldExpiresAt = &exp
		}
		out.Seats = append(out.Seats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if out.Zones, err = r.zones(ctx, eventID); err != nil {
		return nil, err
	}
	if out.Elements, err = r.elements(ctx, eventID); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) zones(ctx context.Context, eventID string) ([]Zone, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT z.code, z.name, z.color, z.price
		FROM admin_hall_seat_zones z
		JOIN admin_hall_seat_maps m ON m.id = z.seat_map_id
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		ORDER BY z.display_order, z.code`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Zone{}
	for rows.Next() {
		var z Zone
		if err := rows.Scan(&z.Code, &z.Name, &z.Color, &z.Price); err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

func (r *Repository) elements(ctx context.Context, eventID string) ([]Element, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT el.kind, el.label, el.pos_x, el.pos_y, el.width, el.height, el.rotation, el.color
		FROM admin_hall_seat_elements el
		JOIN admin_hall_seat_maps m ON m.id = el.seat_map_id
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		ORDER BY el.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Element{}
	for rows.Next() {
		var e Element
		if err := rows.Scan(&e.Kind, &e.Label, &e.PosX, &e.PosY,
			&e.Width, &e.Height, &e.Rotation, &e.Color); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// -----------------------------------------------------------------------------
// Holding seats — the write side of the concurrency model
// -----------------------------------------------------------------------------

// HoldSeats claims seats for a family, replacing any previous hold this family
// had on the event.
//
// THE RACE. Two buyers clicking the same seat at the same moment both reach
// this function. It is safe because:
//
//  1. every seat gets exactly one row in event_seat_status (UNIQUE on
//     (event_id, seat_id)), materialized here if missing;
//  2. those rows are then locked with FOR UPDATE **ordered by seat_id**, so two
//     overlapping requests queue behind each other in the same order instead of
//     deadlocking on a crossed pair;
//  3. the state is re-read *after* the lock is granted, so the loser sees what
//     the winner just wrote and is told which seats went.
//
// The first transaction to commit owns the seats. There is no window in which
// both callers succeed.
//
// On conflict it returns the conflicting seats (not an error) so the caller can
// tell the buyer exactly what to re-pick.
func (r *Repository) HoldSeats(ctx context.Context, eventID, familyID string, seatIDs []string,
	groupID string, expiresAt time.Time, now time.Time, grace time.Duration,
) (seats []SeatPrice, conflicts []SeatConflict, err error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Only seats that are actually sellable and belong to THIS event's hall may
	// be claimed. A seat id from another venue simply resolves to nothing.
	var sellable int
	if err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM admin_hall_seats s
		JOIN admin_hall_seat_maps m ON m.id = s.seat_map_id AND m.status = 'published'
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		WHERE s.id = ANY($2::uuid[]) AND s.is_active AND s.kind <> 'house'`,
		eventID, seatIDs).Scan(&sellable); err != nil {
		return nil, nil, err
	}
	if sellable != len(seatIDs) {
		return nil, nil, httpx.ErrBadRequest(
			"one or more of those seats are not on sale for this event")
	}

	// (1) Materialize a state row per requested seat.
	if _, err = tx.Exec(ctx, `
		INSERT INTO event_seat_status (event_id, seat_id, status)
		SELECT $1, s.id, 'available'
		FROM admin_hall_seats s
		WHERE s.id = ANY($2::uuid[])
		ON CONFLICT (event_id, seat_id) DO NOTHING`, eventID, seatIDs); err != nil {
		return nil, nil, err
	}

	// (2) Lock them in a deterministic order.
	rows, err := tx.Query(ctx, `
		SELECT ess.seat_id, s.seat_label, ess.status,
		       COALESCE(ess.held_by::text, ''), COALESCE(ess.hold_group_id::text, ''),
		       ess.hold_expires_at,
		       COALESCE((
		           SELECT true FROM bookings b
		           WHERE b.hold_group_id = ess.hold_group_id
		             AND b.status = 'payment_processing'
		           LIMIT 1
		       ), false)
		FROM event_seat_status ess
		JOIN admin_hall_seats s ON s.id = ess.seat_id
		WHERE ess.event_id = $1 AND ess.seat_id = ANY($2::uuid[])
		ORDER BY ess.seat_id
		FOR UPDATE OF ess`, eventID, seatIDs)
	if err != nil {
		return nil, nil, err
	}

	// (3) Re-check under the lock.
	type locked struct {
		seatID, label string
		state         SeatState
	}
	var all []locked
	for rows.Next() {
		var l locked
		var expires *time.Time
		if err = rows.Scan(&l.seatID, &l.label, &l.state.Status, &l.state.HeldBy,
			&l.state.HoldGroupID, &expires, &l.state.PaymentInFlight); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if expires != nil {
			l.state.HoldExpiresAt = *expires
		}
		all = append(all, l)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	for _, l := range all {
		if CanClaim(l.state, familyID, now, grace) {
			continue
		}
		reason := AvailHeld
		if l.state.Status == SeatBooked {
			reason = AvailBooked
		}
		conflicts = append(conflicts, SeatConflict{
			SeatID: l.seatID, SeatLabel: l.label, Reason: reason,
		})
	}
	if len(conflicts) > 0 {
		// Nothing is written: the buyer re-picks against fresh state.
		return nil, conflicts, nil
	}

	// A new selection replaces the family's previous one on this event, so a
	// buyer who deselects a seat releases it instead of hoarding it.
	if _, err = tx.Exec(ctx, `
		UPDATE event_seat_status
		SET status='available', held_by=NULL, hold_group_id=NULL,
		    hold_expires_at=NULL, updated_at=now()
		WHERE event_id=$1 AND held_by=$2 AND status='held'
		  AND seat_id <> ALL($3::uuid[])`, eventID, familyID, seatIDs); err != nil {
		return nil, nil, err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE event_seat_status
		SET status='held', held_by=$2, hold_group_id=$3,
		    hold_expires_at=$4, booking_id=NULL, updated_at=now()
		WHERE event_id=$1 AND seat_id = ANY($5::uuid[])`,
		eventID, familyID, groupID, expiresAt, seatIDs); err != nil {
		return nil, nil, err
	}

	// Price the selection from the arrangement — the seat map is the pricing
	// reference, so the buyer is charged exactly what Ops configured.
	priceRows, err := tx.Query(ctx, `
		SELECT s.id, s.seat_label, COALESCE(s.price, z.price, 0)
		FROM admin_hall_seats s
		LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id
		WHERE s.id = ANY($1::uuid[])
		ORDER BY s.section, s.row_index, s.seat_number`, seatIDs)
	if err != nil {
		return nil, nil, err
	}
	for priceRows.Next() {
		var sp SeatPrice
		if err = priceRows.Scan(&sp.SeatID, &sp.SeatLabel, &sp.Price); err != nil {
			priceRows.Close()
			return nil, nil, err
		}
		seats = append(seats, sp)
	}
	priceRows.Close()
	if err = priceRows.Err(); err != nil {
		return nil, nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return seats, nil, nil
}

// ReleaseGroup drops a family's hold. Scoped by family so one buyer can never
// release another's seats.
func (r *Repository) ReleaseGroup(ctx context.Context, eventID, familyID, groupID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE event_seat_status
		SET status='available', held_by=NULL, hold_group_id=NULL,
		    hold_expires_at=NULL, updated_at=now()
		WHERE event_id=$1 AND held_by=$2 AND hold_group_id=$3 AND status='held'`,
		eventID, familyID, groupID)
	return err
}

// HeldSeatsForGroup returns the seats a hold group still owns, priced. An empty
// result means the hold lapsed or was taken over.
func (r *Repository) HeldSeatsForGroup(ctx context.Context, familyID, groupID string,
	now time.Time, grace time.Duration) (eventID string, seats []SeatPrice, expires time.Time, err error) {

	rows, err := r.pool.Query(ctx, `
		SELECT ess.event_id::text, s.id, s.seat_label,
		       COALESCE(s.price, z.price, 0), ess.hold_expires_at, ess.status,
		       COALESCE((
		           SELECT true FROM bookings b
		           WHERE b.hold_group_id = ess.hold_group_id
		             AND b.status = 'payment_processing'
		           LIMIT 1
		       ), false)
		FROM event_seat_status ess
		JOIN admin_hall_seats s ON s.id = ess.seat_id
		LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id
		WHERE ess.hold_group_id = $1 AND ess.held_by = $2
		ORDER BY s.section, s.row_index, s.seat_number`, groupID, familyID)
	if err != nil {
		return "", nil, time.Time{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var sp SeatPrice
		var st SeatState
		var exp *time.Time
		if err = rows.Scan(&eventID, &sp.SeatID, &sp.SeatLabel, &sp.Price,
			&exp, &st.Status, &st.PaymentInFlight); err != nil {
			return "", nil, time.Time{}, err
		}
		if exp != nil {
			st.HoldExpiresAt = *exp
			expires = *exp
		}
		st.HeldBy = familyID
		// Time is the arbiter: a row still flagged 'held' but past its deadline
		// is not a hold, whether or not the sweeper has been round yet.
		if !HoldActive(st, now, grace) {
			return "", nil, expires, nil
		}
		seats = append(seats, sp)
	}
	return eventID, seats, expires, rows.Err()
}

// -----------------------------------------------------------------------------
// Bookings
// -----------------------------------------------------------------------------

// NewGroupID returns a random UUID for a hold group.
//
// Written by hand rather than pulling in a UUID dependency: this is the only
// place the project needs to MINT one (Postgres generates the rest via
// gen_random_uuid()), and the format has to be exact because the column is
// typed uuid.
func NewGroupID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// newReference generates a collision-resistant booking reference.
func newReference() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return FormatReference(b), nil
}

// CreateBooking turns a live hold into a booking, snapshotting each seat.
//
// The seats stay HELD rather than becoming booked: nothing is sold until the
// money arrives. The booking simply inherits the hold's deadline, so the whole
// pick → checkout → pay journey runs inside the one configured window.
func (r *Repository) CreateBooking(ctx context.Context, eventID, familyID, groupID string,
	seats []SeatPrice, totals Totals, couponCode string, expiresAt time.Time,
) (string, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Reusing an existing pending booking for the same hold keeps a buyer who
	// reloads checkout from stacking duplicate bookings on one hold.
	var existing string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM bookings
		WHERE hold_group_id=$1 AND family_id=$2
		  AND status IN ('pending_payment','payment_processing')
		LIMIT 1`, groupID, familyID).Scan(&existing)
	if err == nil && existing != "" {
		if _, err = tx.Exec(ctx, `
			UPDATE bookings SET seat_count=$2, subtotal=$3, discount=$4, total=$5,
			       coupon_code=$6, updated_at=now()
			WHERE id=$1`, existing, totals.SeatCount, totals.Subtotal,
			totals.Discount, totals.Total, couponCode); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM booking_items WHERE booking_id=$1`, existing); err != nil {
			return "", err
		}
		if err = insertItems(ctx, tx, existing, seats); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	ref, err := newReference()
	if err != nil {
		return "", err
	}

	var id string
	if err = tx.QueryRow(ctx, `
		INSERT INTO bookings (reference, event_id, family_id, hold_group_id, status,
		                      seat_count, subtotal, discount, total, coupon_code, hold_expires_at)
		VALUES ($1,$2,$3,$4,'pending_payment',$5,$6,$7,$8,$9,$10)
		RETURNING id::text`,
		ref, eventID, familyID, groupID, totals.SeatCount,
		totals.Subtotal, totals.Discount, totals.Total, couponCode, expiresAt).Scan(&id); err != nil {
		return "", err
	}
	if err = insertItems(ctx, tx, id, seats); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// insertItems snapshots the seats onto the booking. The seat's descriptive
// fields are copied rather than referenced, so a later redesign of the hall
// cannot rewrite what a buyer was sold.
func insertItems(ctx context.Context, tx pgx.Tx, bookingID string, seats []SeatPrice) error {
	if len(seats) == 0 {
		return nil
	}
	ids := make([]string, len(seats))
	prices := make([]float64, len(seats))
	for i, s := range seats {
		ids[i], prices[i] = s.SeatID, s.Price
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO booking_items
			(booking_id, seat_id, section, row_label, row_index, seat_number,
			 seat_label, zone_code, zone_name, price)
		SELECT $1, s.id, s.section, s.row_label, s.row_index, s.seat_number,
		       s.seat_label, COALESCE(z.code,''), COALESCE(z.name,''), k.price
		FROM unnest($2::uuid[], $3::numeric[]) AS k(seat_id, price)
		JOIN admin_hall_seats s ON s.id = k.seat_id
		LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id`,
		bookingID, ids, prices)
	return err
}

// bookingSelect is shared by the list and detail reads so both always expose
// the same fields.
const bookingSelect = `
	SELECT b.id::text, b.reference, b.event_id::text, e.name, e.city, e.start_date,
	       COALESCE(h.name, ''), b.status, b.seat_count, b.subtotal, b.discount, b.total,
	       b.coupon_code, b.hold_expires_at, b.created_at, b.confirmed_at, b.cancelled_at,
	       b.cancellation_reason, b.cancellation_note, b.cancelled_by,
	       b.refund_status, b.refund_amount, b.refund_note,
	       (SELECT count(*) FROM booking_payments p WHERE p.booking_id = b.id)
	FROM bookings b
	JOIN events e ON e.id = b.event_id
	LEFT JOIN admin_halls h ON h.id = e.hall_id`

func scanBooking(row pgx.Row) (*Booking, error) {
	var b Booking
	if err := row.Scan(&b.ID, &b.Reference, &b.EventID, &b.EventName, &b.EventCity,
		&b.EventStart, &b.HallName, &b.Status, &b.SeatCount, &b.Subtotal, &b.Discount,
		&b.Total, &b.CouponCode, &b.HoldExpiresAt, &b.CreatedAt, &b.ConfirmedAt,
		&b.CancelledAt, &b.CancellationReason, &b.CancellationNote, &b.CancelledBy,
		&b.RefundStatus, &b.RefundAmount, &b.RefundNote, &b.PaymentAttempts); err != nil {
		return nil, err
	}
	if label, ok := CancellationReasons[b.CancellationReason]; ok {
		b.CancellationReasonLabel = label
	}
	return &b, nil
}

// ListBookings returns a family's booking history, newest first. Cancelled and
// expired bookings are included — the history is the point.
func (r *Repository) ListBookings(ctx context.Context, familyID string) ([]Booking, error) {
	rows, err := r.pool.Query(ctx, bookingSelect+`
		WHERE b.family_id = $1 ORDER BY b.created_at DESC`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Booking{}
	ids := []string{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
		ids = append(ids, b.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// One extra query for every booking's seats, rather than N.
	if len(ids) > 0 {
		items, err := r.itemsFor(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Items = items[out[i].ID]
		}
	}
	return out, nil
}

func (r *Repository) itemsFor(ctx context.Context, bookingIDs []string) (map[string][]BookingItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT booking_id::text, id::text, COALESCE(seat_id::text,''), section, row_label,
		       row_index, seat_number, seat_label, zone_code, zone_name, price, status
		FROM booking_items
		WHERE booking_id = ANY($1::uuid[])
		ORDER BY section, row_index, seat_number`, bookingIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]BookingItem{}
	for rows.Next() {
		var bid string
		var it BookingItem
		if err := rows.Scan(&bid, &it.ID, &it.SeatID, &it.Section, &it.RowLabel,
			&it.RowIndex, &it.SeatNumber, &it.SeatLabel, &it.ZoneCode, &it.ZoneName,
			&it.Price, &it.Status); err != nil {
			return nil, err
		}
		out[bid] = append(out[bid], it)
	}
	return out, rows.Err()
}

// GetBooking loads one booking, scoped to its owner. Scoping in the WHERE
// clause (rather than checking after) means another family's booking is simply
// not found — it never leaks that it exists.
func (r *Repository) GetBooking(ctx context.Context, id, familyID string) (*Booking, error) {
	b, err := scanBooking(r.pool.QueryRow(ctx, bookingSelect+`
		WHERE b.id = $1 AND b.family_id = $2`, id, familyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrNotFound("booking not found")
	}
	if err != nil {
		return nil, err
	}
	items, err := r.itemsFor(ctx, []string{b.ID})
	if err != nil {
		return nil, err
	}
	b.Items = items[b.ID]
	return b, nil
}

// -----------------------------------------------------------------------------
// Payment
// -----------------------------------------------------------------------------

// StartPayment records an attempt and moves the booking to payment_processing,
// which is what buys it the grace window.
//
// The booking row is locked FOR UPDATE so two clicks on "Pay" cannot open two
// authorisations for the same money.
func (r *Repository) StartPayment(ctx context.Context, bookingID, familyID string,
	now time.Time) (*PaymentIntent, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var total float64
	var expires time.Time
	if err = tx.QueryRow(ctx, `
		SELECT status, total, hold_expires_at FROM bookings
		WHERE id=$1 AND family_id=$2 FOR UPDATE`,
		bookingID, familyID).Scan(&status, &total, &expires); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound("booking not found")
		}
		return nil, err
	}

	var attempts int
	if err = tx.QueryRow(ctx,
		`SELECT count(*) FROM booking_payments WHERE booking_id=$1`, bookingID).Scan(&attempts); err != nil {
		return nil, err
	}

	// An authorisation is already open — hand back the same one rather than
	// charging twice.
	if status == StatusPaymentProcessing {
		var ref string
		var amount float64
		var attempt int
		if err = tx.QueryRow(ctx, `
			SELECT provider_ref, amount, attempt FROM booking_payments
			WHERE booking_id=$1 AND status='processing'
			ORDER BY created_at DESC LIMIT 1`, bookingID).Scan(&ref, &amount, &attempt); err == nil {
			if err = tx.Commit(ctx); err != nil {
				return nil, err
			}
			return &PaymentIntent{
				BookingID: bookingID, ProviderRef: ref, Provider: "mock",
				Amount: amount, Attempt: attempt, ExpiresAt: expires,
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	if ok, why := CanStartPayment(PayabilityInput{
		Status: status, HoldExpiresAt: expires, Attempts: attempts,
	}, now); !ok {
		return nil, httpx.ErrConflict(why)
	}

	ref := fmt.Sprintf("pay_%s_%d", bookingID[:8], now.UnixNano())
	if _, err = tx.Exec(ctx, `
		INSERT INTO booking_payments (booking_id, provider, provider_ref, amount, attempt, status)
		VALUES ($1,'mock',$2,$3,$4,'processing')`,
		bookingID, ref, total, attempts+1); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE bookings SET status='payment_processing', updated_at=now() WHERE id=$1`,
		bookingID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PaymentIntent{
		BookingID: bookingID, ProviderRef: ref, Provider: "mock",
		Amount: total, Attempt: attempts + 1, ExpiresAt: expires,
	}, nil
}

// ApplyConfirm settles a payment attempt.
//
// The whole decision is made by DecideConfirm (pure, unit-tested); this method
// only applies it. The critical step is the seat re-check under lock: if the
// money arrived but the seats are gone, the booking is cancelled with a refund
// owed rather than sold twice.
func (r *Repository) ApplyConfirm(ctx context.Context, bookingID, familyID, providerRef string,
	success bool, now time.Time, grace time.Duration) (*ConfirmResult, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, groupID, eventID string
	var total float64
	var expires time.Time
	if err = tx.QueryRow(ctx, `
		SELECT status, hold_group_id::text, event_id::text, total, hold_expires_at
		FROM bookings WHERE id=$1 AND family_id=$2 FOR UPDATE`,
		bookingID, familyID).Scan(&status, &groupID, &eventID, &total, &expires); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound("booking not found")
		}
		return nil, err
	}

	var attempts int
	if err = tx.QueryRow(ctx,
		`SELECT count(*) FROM booking_payments WHERE booking_id=$1`, bookingID).Scan(&attempts); err != nil {
		return nil, err
	}

	// Are the seats still ours? Locking them here also stops anyone else
	// claiming them while we decide.
	var seatsHeld, seatCount int
	if err = tx.QueryRow(ctx, `
		SELECT count(*) FROM booking_items WHERE booking_id=$1`, bookingID).Scan(&seatCount); err != nil {
		return nil, err
	}
	if err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM event_seat_status ess
		WHERE ess.event_id=$1 AND ess.hold_group_id=$2 AND ess.held_by=$3
		  AND ess.status='held'`, eventID, groupID, familyID).Scan(&seatsHeld); err != nil {
		return nil, err
	}

	outcome := DecideConfirm(ConfirmInput{
		Status: status, HoldExpiresAt: expires, Attempts: attempts,
		PaymentSucceeded: success,
		SeatsStillHeld:   seatCount > 0 && seatsHeld == seatCount,
	}, now, grace)

	res := &ConfirmResult{Message: outcome.Reason, AttemptsLeft: MaxPaymentAttempts - attempts}

	switch {
	case outcome.AlreadyDone:
		res.Outcome = "already_confirmed"
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}

	case outcome.Confirm:
		if _, err = tx.Exec(ctx, `
			UPDATE booking_payments SET status='succeeded', settled_at=now()
			WHERE booking_id=$1 AND provider_ref=$2`, bookingID, providerRef); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE bookings SET status='confirmed', confirmed_at=now(), updated_at=now()
			WHERE id=$1`, bookingID); err != nil {
			return nil, err
		}
		// The seats are sold: this is the only place status becomes 'booked'.
		if _, err = tx.Exec(ctx, `
			UPDATE event_seat_status
			SET status='booked', booking_id=$3, hold_expires_at=NULL, updated_at=now()
			WHERE event_id=$1 AND hold_group_id=$2`, eventID, groupID, bookingID); err != nil {
			return nil, err
		}
		res.Outcome = "confirmed"
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}

	case outcome.CancelWithRefund:
		plan := PlanRefund(StatusConfirmed, total) // money was taken, so it is owed back
		if _, err = tx.Exec(ctx, `
			UPDATE booking_payments SET status='succeeded', settled_at=now()
			WHERE booking_id=$1 AND provider_ref=$2`, bookingID, providerRef); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE bookings SET status='cancelled', cancelled_at=now(), cancelled_by='system',
			       cancellation_reason='other',
			       cancellation_note='Seats were released before the payment completed',
			       refund_status=$2, refund_amount=$3, updated_at=now()
			WHERE id=$1`, bookingID, plan.Status, plan.Amount); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx,
			`UPDATE booking_items SET status='cancelled' WHERE booking_id=$1`, bookingID); err != nil {
			return nil, err
		}
		res.Outcome = "cancelled_refund_due"
		res.Refund = &plan
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}

	case outcome.Expire:
		if _, err = tx.Exec(ctx, `
			UPDATE booking_payments SET status='failed', failure_reason=$3, settled_at=now()
			WHERE booking_id=$1 AND provider_ref=$2`,
			bookingID, providerRef, outcome.Reason); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE bookings SET status='expired', updated_at=now() WHERE id=$1`, bookingID); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE event_seat_status
			SET status='available', held_by=NULL, hold_group_id=NULL,
			    hold_expires_at=NULL, updated_at=now()
			WHERE event_id=$1 AND hold_group_id=$2 AND status='held'`,
			eventID, groupID); err != nil {
			return nil, err
		}
		res.Outcome = "expired"
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}

	default: // retry
		if _, err = tx.Exec(ctx, `
			UPDATE booking_payments SET status='failed', failure_reason='declined', settled_at=now()
			WHERE booking_id=$1 AND provider_ref=$2`, bookingID, providerRef); err != nil {
			return nil, err
		}
		// Back to pending so the buyer can try again on the same held seats.
		if _, err = tx.Exec(ctx, `
			UPDATE bookings SET status='pending_payment', updated_at=now() WHERE id=$1`,
			bookingID); err != nil {
			return nil, err
		}
		res.Outcome = "retry"
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
	}

	if b, err := r.GetBooking(ctx, bookingID, familyID); err == nil {
		res.Booking = b
	}
	return res, nil
}

// -----------------------------------------------------------------------------
// Cancellation
// -----------------------------------------------------------------------------

// Cancel cancels a confirmed booking, frees its seats and records what is owed.
//
// Nothing is deleted: the booking, its seats and the reason all remain so the
// family keeps a complete history. Refunds are settled offline, so this only
// files the work item.
func (r *Repository) Cancel(ctx context.Context, bookingID, familyID, reason, note string,
	now time.Time) (*CancelResult, error) {

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, groupID, eventID string
	var total float64
	var eventStart time.Time
	if err = tx.QueryRow(ctx, `
		SELECT b.status, b.hold_group_id::text, b.event_id::text, b.total, e.start_date
		FROM bookings b JOIN events e ON e.id = b.event_id
		WHERE b.id=$1 AND b.family_id=$2 FOR UPDATE OF b`,
		bookingID, familyID).Scan(&status, &groupID, &eventID, &total, &eventStart); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound("booking not found")
		}
		return nil, err
	}

	// The event's live start date is used, not a copy taken at purchase, so a
	// rescheduled event moves the cancellation window with it.
	if ok, why := CanCancel(CancelCheck{
		Status: status, EventStart: eventStart, Total: total,
	}, now); !ok {
		return nil, httpx.ErrConflict(why)
	}

	plan := PlanRefund(status, total)

	if _, err = tx.Exec(ctx, `
		UPDATE bookings SET status='cancelled', cancelled_at=now(), cancelled_by='family',
		       cancellation_reason=$2, cancellation_note=$3,
		       refund_status=$4, refund_amount=$5, updated_at=now()
		WHERE id=$1`, bookingID, reason, note, plan.Status, plan.Amount); err != nil {
		return nil, err
	}
	// Items are marked cancelled rather than removed — this is the historical
	// record of what was given up.
	if _, err = tx.Exec(ctx,
		`UPDATE booking_items SET status='cancelled' WHERE booking_id=$1`, bookingID); err != nil {
		return nil, err
	}
	// Put the seats straight back on sale.
	if _, err = tx.Exec(ctx, `
		UPDATE event_seat_status
		SET status='available', held_by=NULL, hold_group_id=NULL, hold_expires_at=NULL,
		    booking_id=NULL, updated_at=now()
		WHERE event_id=$1 AND booking_id=$2`, eventID, bookingID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}

	b, err := r.GetBooking(ctx, bookingID, familyID)
	if err != nil {
		return nil, err
	}
	return &CancelResult{Booking: b, Refund: plan}, nil
}

// -----------------------------------------------------------------------------
// Sweeper
// -----------------------------------------------------------------------------

// SweepExpired tidies up lapsed holds and stale bookings.
//
// This is HOUSEKEEPING, not the rule: reads and claims already judge expiry by
// time, so availability is correct even if this never runs. Its value is
// keeping the tables honest for reporting and freeing rows promptly.
//
// A hold whose booking is mid-payment is skipped until its grace window closes
// — releasing it would risk selling a seat someone is being charged for.
func (r *Repository) SweepExpired(ctx context.Context, now time.Time,
	grace time.Duration) (holds int64, bookings int64, err error) {

	ct, err := r.pool.Exec(ctx, `
		UPDATE event_seat_status ess
		SET status='available', held_by=NULL, hold_group_id=NULL,
		    hold_expires_at=NULL, updated_at=now()
		WHERE ess.status='held'
		  AND ess.hold_expires_at < $1
		  AND NOT EXISTS (
		      SELECT 1 FROM bookings b
		      WHERE b.hold_group_id = ess.hold_group_id
		        AND b.status = 'payment_processing'
		        AND b.hold_expires_at + $2::interval > $1
		  )`, now, grace.String())
	if err != nil {
		return 0, 0, err
	}
	holds = ct.RowsAffected()

	ct, err = r.pool.Exec(ctx, `
		UPDATE bookings SET status='expired', updated_at=now()
		WHERE (status='pending_payment' AND hold_expires_at < $1)
		   OR (status='payment_processing' AND hold_expires_at + $2::interval < $1)`,
		now, grace.String())
	if err != nil {
		return holds, 0, err
	}
	return holds, ct.RowsAffected(), nil
}
