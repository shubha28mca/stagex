package operationaladmin

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Seat reservation report — Operational Admin.
//
// Answers three questions Ops cannot get anywhere else:
//
//   1. seat by seat, what is available, paid for, being paid for, or blocked;
//   2. per price tier, how much of the possible money was actually taken;
//   3. what the event's profit and loss is INCLUDING ticket sales.
//
// The occupancy verdict is resolved by OccupancyStatus (seatreport_logic.go),
// which applies the same time-based expiry rule as the participant API — so
// Ops and buyers can never disagree about whether a seat is taken. Unlike the
// buyer's view, this one also surfaces blocked and house seats, because those
// are exactly the ones Ops needs when arranging a hall.

// The payment grace must match the participant API's PAYMENT_GRACE. It only
// affects whether a seat mid-authorisation reads as held, so a small mismatch
// is cosmetic rather than dangerous — but keep them in step.
const reportPaymentGrace = 5 * time.Minute

// SeatReport is the whole seat-reservation report for one event.
type SeatReport struct {
	EventID     string    `json:"eventId"`
	EventName   string    `json:"eventName"`
	City        string    `json:"city"`
	StartDate   time.Time `json:"startDate"`
	EventStatus string    `json:"eventStatus"`
	HallName    string    `json:"hallName"`
	// HallCapacity is the registry figure, kept beside the mapped seat count so
	// a stale capacity is obvious.
	HallCapacity int       `json:"hallCapacity"`
	MapStatus    string    `json:"mapStatus"`
	GeneratedAt  time.Time `json:"generatedAt"`

	CanvasWidth  int `json:"canvasWidth"`
	CanvasHeight int `json:"canvasHeight"`

	Summary SeatSummary     `json:"summary"`
	ByZone  []ZoneRevenue   `json:"byZone"`
	Seats   []SeatOccupancy `json:"seats"`
	// Elements let the console draw the same plan the designer produced.
	Elements []reportElement `json:"elements"`

	Tickets       TicketFinance             `json:"tickets"`
	Cancellations []CancellationReasonCount `json:"cancellations"`
	PnL           PnL                       `json:"pnl"`
}

type reportElement struct {
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	PosX   int    `json:"posX"`
	PosY   int    `json:"posY"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Color  string `json:"color"`
}

type seatReportService struct{ pool *pgxpool.Pool }

// header loads the event, its hall and the published arrangement.
func (s seatReportService) header(ctx context.Context, eventID string) (*SeatReport, error) {
	var r SeatReport
	err := s.pool.QueryRow(ctx, `
		SELECT e.id::text, e.name, e.city, e.start_date, e.status,
		       COALESCE(h.name,''), COALESCE(h.capacity,0),
		       COALESCE(m.status,''), COALESCE(m.canvas_width,1200), COALESCE(m.canvas_height,800)
		FROM events e
		LEFT JOIN admin_halls h ON h.id = e.hall_id
		LEFT JOIN admin_hall_seat_maps m ON m.hall_id = h.id
		WHERE e.id = $1`, eventID).
		Scan(&r.EventID, &r.EventName, &r.City, &r.StartDate, &r.EventStatus,
			&r.HallName, &r.HallCapacity, &r.MapStatus, &r.CanvasWidth, &r.CanvasHeight)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrNotFound("event not found")
	}
	if err != nil {
		return nil, err
	}
	if r.MapStatus == "" {
		return nil, httpx.ErrConflict(
			"this event's hall has no seating arrangement — design one under Seat Maps first")
	}
	r.GeneratedAt = time.Now()
	return &r, nil
}

// seats loads every seat with its occupancy and, when sold, who bought it.
//
// Blocked (is_active = false) and house seats are INCLUDED here — that is the
// difference from the buyer's view, and the whole point of the report.
func (s seatReportService) seats(ctx context.Context, eventID string) ([]SeatOccupancy, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.id::text, s.section, s.row_label, s.row_index, s.seat_number, s.seat_label,
		       COALESCE(z.code,''), COALESCE(z.name,''), COALESCE(z.color,'#c9c4d8'),
		       s.kind, s.pos_x, s.pos_y, s.is_active,
		       COALESCE(s.price, z.price, 0)                        AS list_price,
		       COALESCE(ess.status, 'available')                    AS ess_status,
		       ess.hold_expires_at,
		       -- Is the hold on this seat backed by a payment in flight? Drives
		       -- both the grace window and the "someone is paying right now"
		       -- flag Ops needs when a hall is nearly full.
		       COALESCE((
		           SELECT true FROM bookings hb
		           WHERE hb.hold_group_id = ess.hold_group_id
		             AND hb.status = 'payment_processing' LIMIT 1
		       ), false),
		       -- Who holds the seat, if it is sold. Joined through the booking
		       -- rather than the item snapshot so a cancelled booking's old
		       -- seats never appear as sold.
		       COALESCE(b.id::text,''), COALESCE(b.reference,''),
		       COALESCE(f.display_name,''), COALESCE(f.phone,''),
		       COALESCE(bi.price, 0), b.confirmed_at
		FROM admin_hall_seats s
		JOIN admin_hall_seat_maps m ON m.id = s.seat_map_id
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id
		LEFT JOIN event_seat_status ess ON ess.event_id = e.id AND ess.seat_id = s.id
		LEFT JOIN bookings b ON b.id = ess.booking_id AND b.status = 'confirmed'
		LEFT JOIN booking_items bi ON bi.booking_id = b.id AND bi.seat_id = s.id
		LEFT JOIN families f ON f.id = b.family_id
		ORDER BY s.section, s.row_index, s.seat_number`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	out := []SeatOccupancy{}
	for rows.Next() {
		var o SeatOccupancy
		var f SeatFacts
		var holdExpires, bookedAt *time.Time
		if err := rows.Scan(&o.SeatID, &o.Section, &o.RowLabel, &o.RowIndex, &o.SeatNumber,
			&o.SeatLabel, &o.ZoneCode, &o.ZoneName, &o.ZoneColor, &o.Kind, &o.PosX, &o.PosY,
			&f.IsActive, &o.ListPrice, &f.ESSStatus, &holdExpires, &f.PaymentInFlight,
			&o.BookingID, &o.BookingRef, &o.BuyerName, &o.BuyerPhone,
			&o.PricePaid, &bookedAt); err != nil {
			return nil, err
		}
		f.Kind = o.Kind
		if holdExpires != nil {
			f.HoldExpiresAt = *holdExpires
		}
		o.Status = OccupancyStatus(f, now, reportPaymentGrace)

		// Only attach the detail that belongs to the resolved status, so a
		// lapsed hold cannot leave stale buyer data on an available seat.
		switch o.Status {
		case OccBooked:
			o.BookedAt = bookedAt
		case OccHeld:
			o.HoldExpiresAt = holdExpires
			o.PaymentInFlight = f.PaymentInFlight
			o.BookingID, o.BookingRef = "", ""
			o.BuyerName, o.BuyerPhone, o.PricePaid = "", "", 0
		default:
			o.BookingID, o.BookingRef = "", ""
			o.BuyerName, o.BuyerPhone, o.PricePaid = "", "", 0
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s seatReportService) zones(ctx context.Context, eventID string) ([]ZoneRevenue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT z.code, z.name, z.color
		FROM admin_hall_seat_zones z
		JOIN admin_hall_seat_maps m ON m.id = z.seat_map_id
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		ORDER BY z.display_order, z.code`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ZoneRevenue{}
	for rows.Next() {
		var z ZoneRevenue
		if err := rows.Scan(&z.Code, &z.Name, &z.Color); err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

func (s seatReportService) elements(ctx context.Context, eventID string) ([]reportElement, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT el.kind, el.label, el.pos_x, el.pos_y, el.width, el.height, el.color
		FROM admin_hall_seat_elements el
		JOIN admin_hall_seat_maps m ON m.id = el.seat_map_id
		JOIN events e ON e.hall_id = m.hall_id AND e.id = $1
		ORDER BY el.created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []reportElement{}
	for rows.Next() {
		var e reportElement
		if err := rows.Scan(&e.Kind, &e.Label, &e.PosX, &e.PosY, &e.Width, &e.Height, &e.Color); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ticketFinance sums the ticket money for an event.
//
// Gross counts every seat that was successfully paid for, INCLUDING seats on
// bookings later cancelled — because that money genuinely arrived. Refunds are
// then subtracted. Netting this way keeps the P&L reconcilable against the
// bank instead of quietly dropping cancelled sales.
func (s seatReportService) ticketFinance(ctx context.Context, eventID string) (*TicketFinance, error) {
	var t TicketFinance
	err := s.pool.QueryRow(ctx, `
		SELECT
		  -- gross: seats on any booking that reached a successful payment
		  COALESCE((
		    SELECT SUM(bi.price) FROM booking_items bi
		    JOIN bookings b ON b.id = bi.booking_id
		    WHERE b.event_id = $1
		      AND (b.status = 'confirmed'
		           OR (b.status = 'cancelled' AND b.refund_status <> 'not_applicable'))
		  ), 0),
		  COALESCE((
		    SELECT SUM(b.refund_amount) FROM bookings b
		    WHERE b.event_id = $1 AND b.refund_status <> 'not_applicable'
		  ), 0),
		  COALESCE((
		    SELECT SUM(b.refund_amount) FROM bookings b
		    WHERE b.event_id = $1 AND b.refund_status = 'pending_offline'
		  ), 0),
		  (SELECT count(*) FROM booking_items bi JOIN bookings b ON b.id = bi.booking_id
		     WHERE b.event_id = $1 AND b.status = 'confirmed' AND bi.status = 'confirmed'),
		  (SELECT count(*) FROM booking_items bi JOIN bookings b ON b.id = bi.booking_id
		     WHERE b.event_id = $1 AND bi.status = 'cancelled'),
		  (SELECT count(*) FROM bookings WHERE event_id = $1 AND status = 'confirmed'),
		  (SELECT count(*) FROM bookings WHERE event_id = $1 AND status = 'cancelled')`,
		eventID).Scan(&t.Gross, &t.Refunds, &t.RefundsPending,
		&t.TicketsSold, &t.TicketsCancelled, &t.Bookings, &t.CancelledBookings)
	if err != nil {
		return nil, err
	}
	out := ComputeTicketFinance(t)
	return &out, nil
}

// cancellations answers "why are people giving tickets back?".
func (s seatReportService) cancellations(ctx context.Context, eventID string) ([]CancellationReasonCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(NULLIF(cancellation_reason,''),'unspecified'),
		       count(*), COALESCE(SUM(refund_amount),0)
		FROM bookings
		WHERE event_id = $1 AND status = 'cancelled'
		GROUP BY 1`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CancellationReasonCount{}
	for rows.Next() {
		var c CancellationReasonCount
		if err := rows.Scan(&c.Reason, &c.Count, &c.Amount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return SortReasonCounts(out), nil
}

// build assembles the whole report.
func (s seatReportService) build(ctx context.Context, eventID string) (*SeatReport, error) {
	r, err := s.header(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if r.Seats, err = s.seats(ctx, eventID); err != nil {
		return nil, err
	}
	zones, err := s.zones(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if r.Elements, err = s.elements(ctx, eventID); err != nil {
		return nil, err
	}
	r.Summary, r.ByZone = SummarizeSeats(r.Seats, zones)

	tf, err := s.ticketFinance(ctx, eventID)
	if err != nil {
		return nil, err
	}
	r.Tickets = *tf

	if r.Cancellations, err = s.cancellations(ctx, eventID); err != nil {
		return nil, err
	}

	// The full event P&L, which already folds ticket income in via pnl().
	pnl, err := (eventOpsService{s.pool}).pnl(ctx, eventID)
	if err != nil {
		return nil, err
	}
	r.PnL = *pnl
	return r, nil
}

// -----------------------------------------------------------------------------
// Routes
// -----------------------------------------------------------------------------

func registerSeatReport(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := seatReportService{pool}

	// Seat-by-seat occupancy — what is available, paid, held or blocked.
	// Powers the console's seat grid, so it returns the plan geometry too.
	mux.Handle("GET /admin/ops/events/{id}/seat-occupancy", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			data, err := svc.build(r.Context(), r.PathValue("id"))
			respond(w, data, err)
		})))

	// The seat-reservation report, generated on demand.
	//   ?format=csv (default) | pdf | json
	mux.Handle("GET /admin/ops/events/{id}/seat-report", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			d, err := svc.build(r.Context(), r.PathValue("id"))
			if err != nil {
				httpx.Error(w, err)
				return
			}
			switch r.URL.Query().Get("format") {
			case "pdf":
				seatReportPDF(w, d)
			case "json":
				httpx.JSON(w, http.StatusOK, d)
			default:
				seatReportCSV(w, d)
			}
		})))
}

// -----------------------------------------------------------------------------
// Exports
// -----------------------------------------------------------------------------

func pct(v float64) string { return fmt.Sprintf("%.1f%%", v) }

// seatReportCSV writes the report as a spreadsheet: summary and P&L first for
// someone skimming it, then the full seat list for someone reconciling it.
func seatReportCSV(w http.ResponseWriter, d *SeatReport) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="stagex-seat-reservation-report.csv"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"StageX — Seat Reservation Report"})
	_ = cw.Write([]string{"Event", d.EventName})
	_ = cw.Write([]string{"City", d.City})
	_ = cw.Write([]string{"Hall", d.HallName})
	_ = cw.Write([]string{"Starts", d.StartDate.Format("2006-01-02")})
	_ = cw.Write([]string{"Arrangement", d.MapStatus})
	_ = cw.Write([]string{"Generated", d.GeneratedAt.Format("2006-01-02 15:04")})
	_ = cw.Write([]string{})

	_ = cw.Write([]string{"SEAT OCCUPANCY"})
	_ = cw.Write([]string{"Mapped seats", fmt.Sprintf("%d", d.Summary.MappedSeats)})
	_ = cw.Write([]string{"Sellable seats", fmt.Sprintf("%d", d.Summary.Sellable)})
	_ = cw.Write([]string{"Booked (paid)", fmt.Sprintf("%d", d.Summary.Booked)})
	_ = cw.Write([]string{"Held (payment in progress)", fmt.Sprintf("%d", d.Summary.Held)})
	_ = cw.Write([]string{"Available", fmt.Sprintf("%d", d.Summary.Available)})
	_ = cw.Write([]string{"Blocked", fmt.Sprintf("%d", d.Summary.Blocked)})
	_ = cw.Write([]string{"House / press hold", fmt.Sprintf("%d", d.Summary.House)})
	_ = cw.Write([]string{"Hall registry capacity", fmt.Sprintf("%d", d.HallCapacity)})
	_ = cw.Write([]string{"Occupancy (booked / sellable)", pct(d.Summary.OccupancyPct)})
	_ = cw.Write([]string{})

	_ = cw.Write([]string{"SEAT VALUE"})
	_ = cw.Write([]string{"Full-house list value", money(d.Summary.ListValue)})
	_ = cw.Write([]string{"Realised (paid)", money(d.Summary.Realised)})
	_ = cw.Write([]string{"In progress (held)", money(d.Summary.HeldValue)})
	_ = cw.Write([]string{"Still unsold", money(d.Summary.UnsoldValue)})
	_ = cw.Write([]string{"Fill rate (realised / list)", pct(d.Summary.FillRatePct)})
	_ = cw.Write([]string{})

	_ = cw.Write([]string{"BY PRICE TIER"})
	_ = cw.Write([]string{"Tier", "Seats", "Booked", "Held", "Available", "Blocked",
		"List value", "Realised"})
	for _, z := range d.ByZone {
		_ = cw.Write([]string{z.Name,
			fmt.Sprintf("%d", z.Seats), fmt.Sprintf("%d", z.Booked),
			fmt.Sprintf("%d", z.Held), fmt.Sprintf("%d", z.Available),
			fmt.Sprintf("%d", z.Blocked),
			money(z.ListValue), money(z.Realised)})
	}
	_ = cw.Write([]string{})

	_ = cw.Write([]string{"TICKET SALES"})
	_ = cw.Write([]string{"Bookings confirmed", fmt.Sprintf("%d", d.Tickets.Bookings)})
	_ = cw.Write([]string{"Bookings cancelled", fmt.Sprintf("%d", d.Tickets.CancelledBookings)})
	_ = cw.Write([]string{"Tickets sold", fmt.Sprintf("%d", d.Tickets.TicketsSold)})
	_ = cw.Write([]string{"Tickets cancelled", fmt.Sprintf("%d", d.Tickets.TicketsCancelled)})
	_ = cw.Write([]string{"Gross collected", money(d.Tickets.Gross)})
	_ = cw.Write([]string{"Refunds (offline)", money(d.Tickets.Refunds)})
	_ = cw.Write([]string{"  of which still owed", money(d.Tickets.RefundsPending)})
	_ = cw.Write([]string{"Net ticket income", money(d.Tickets.Net)})
	_ = cw.Write([]string{})

	if len(d.Cancellations) > 0 {
		_ = cw.Write([]string{"CANCELLATION REASONS"})
		_ = cw.Write([]string{"Reason", "Bookings", "Refund amount"})
		for _, c := range d.Cancellations {
			_ = cw.Write([]string{c.Reason, fmt.Sprintf("%d", c.Count), money(c.Amount)})
		}
		_ = cw.Write([]string{})
	}

	_ = cw.Write([]string{"PROFIT & LOSS (whole event)"})
	_ = cw.Write([]string{"Registration revenue", money(d.PnL.Revenue)})
	_ = cw.Write([]string{"Ticket income (net of refunds)", money(d.PnL.TicketIncome)})
	_ = cw.Write([]string{"Sponsor income", money(d.PnL.SponsorIncome)})
	_ = cw.Write([]string{"Vendor income", money(d.PnL.VendorIncome)})
	_ = cw.Write([]string{"TOTAL INCOME", money(d.PnL.TotalIncome)})
	_ = cw.Write([]string{"Hall cost", money(d.PnL.HallCost)})
	_ = cw.Write([]string{"Crew cost", money(d.PnL.CrewCost)})
	_ = cw.Write([]string{"Other expenses", money(d.PnL.Expenses)})
	_ = cw.Write([]string{"TOTAL EXPENSES", money(d.PnL.TotalExpenses)})
	_ = cw.Write([]string{"NET PROFIT / LOSS", money(d.PnL.NetPL)})
	_ = cw.Write([]string{"Margin", pct(d.PnL.Margin)})
	_ = cw.Write([]string{"Refund liability outstanding", money(d.PnL.RefundsPending)})
	_ = cw.Write([]string{})

	// The seat-by-seat detail: one row per seat, so the whole hall can be
	// reconciled or arranged from a spreadsheet.
	_ = cw.Write([]string{"SEAT DETAIL"})
	_ = cw.Write([]string{"Section", "Row", "Rows from stage", "Seat", "Label", "Tier",
		"Type", "List price", "Status", "Paid", "Booking ref", "Booked by", "Phone", "Booked at"})
	for _, s := range d.Seats {
		bookedAt := ""
		if s.BookedAt != nil {
			bookedAt = s.BookedAt.Format("2006-01-02 15:04")
		}
		paid := ""
		if s.Status == OccBooked {
			paid = money(s.PricePaid)
		}
		_ = cw.Write([]string{
			s.Section, s.RowLabel, fmt.Sprintf("%d", s.RowIndex),
			fmt.Sprintf("%d", s.SeatNumber), s.SeatLabel,
			s.ZoneName, s.Kind, money(s.ListPrice), s.Status, paid,
			s.BookingRef, s.BuyerName, s.BuyerPhone, bookedAt,
		})
	}
}

// seatReportPDF writes the same report as a one-or-two page printable summary
// plus the sold-seat detail. Available seats are omitted from the PDF's detail
// table — a printout listing 400 empty seats helps nobody; the CSV has them.
func seatReportPDF(w http.ResponseWriter, d *SeatReport) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("StageX Seat Reservation Report", false)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "StageX — Seat Reservation Report", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("%s   %s   %s",
		d.EventName, d.City, d.StartDate.Format("2 Jan 2006")), "", 1, "L", false, 0, "")
	pdf.CellFormat(0, 6, fmt.Sprintf("Hall: %s   Arrangement: %s   Generated: %s",
		d.HallName, d.MapStatus, d.GeneratedAt.Format("2 Jan 2006 15:04")), "", 1, "L", false, 0, "")
	pdf.Ln(3)

	kv := func(label, value string, bold bool) {
		if bold {
			pdf.SetFont("Arial", "B", 10)
		} else {
			pdf.SetFont("Arial", "", 10)
		}
		pdf.CellFormat(85, 6, label, "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, value, "", 1, "L", false, 0, "")
	}
	block := func(title string) {
		pdf.Ln(2)
		pdf.SetFont("Arial", "B", 12)
		pdf.SetFillColor(242, 233, 251)
		pdf.CellFormat(0, 8, title, "", 1, "L", true, 0, "")
	}

	block("Seat occupancy")
	kv("Mapped seats", fmt.Sprintf("%d", d.Summary.MappedSeats), false)
	kv("Sellable seats", fmt.Sprintf("%d", d.Summary.Sellable), false)
	kv("Booked (paid)", fmt.Sprintf("%d", d.Summary.Booked), true)
	kv("Held (payment in progress)", fmt.Sprintf("%d", d.Summary.Held), false)
	kv("Available", fmt.Sprintf("%d", d.Summary.Available), true)
	kv("Blocked", fmt.Sprintf("%d", d.Summary.Blocked), false)
	kv("House / press hold", fmt.Sprintf("%d", d.Summary.House), false)
	kv("Occupancy (booked / sellable)", pct(d.Summary.OccupancyPct), true)

	block("Seat value")
	kv("Full-house list value", money(d.Summary.ListValue), false)
	kv("Realised (paid)", money(d.Summary.Realised), true)
	kv("In progress (held)", money(d.Summary.HeldValue), false)
	kv("Still unsold", money(d.Summary.UnsoldValue), false)
	kv("Fill rate (realised / list)", pct(d.Summary.FillRatePct), true)

	block("By price tier")
	pdf.SetFont("Arial", "B", 9)
	tierW := []float64{45, 20, 20, 18, 22, 33, 33}
	for i, h := range []string{"Tier", "Seats", "Booked", "Held", "Available", "List value", "Realised"} {
		pdf.CellFormat(tierW[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Arial", "", 9)
	for _, z := range d.ByZone {
		for i, c := range []string{z.Name,
			fmt.Sprintf("%d", z.Seats), fmt.Sprintf("%d", z.Booked),
			fmt.Sprintf("%d", z.Held), fmt.Sprintf("%d", z.Available),
			money(z.ListValue), money(z.Realised)} {
			pdf.CellFormat(tierW[i], 6, opsTrunc(c, tierW[i]), "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	block("Ticket sales")
	kv("Tickets sold", fmt.Sprintf("%d", d.Tickets.TicketsSold), true)
	kv("Tickets cancelled", fmt.Sprintf("%d", d.Tickets.TicketsCancelled), false)
	kv("Gross collected", money(d.Tickets.Gross), false)
	kv("Refunds (offline)", money(d.Tickets.Refunds), false)
	kv("   of which still owed", money(d.Tickets.RefundsPending), false)
	kv("Net ticket income", money(d.Tickets.Net), true)

	block("Profit & loss (whole event)")
	kv("Registration revenue", money(d.PnL.Revenue), false)
	kv("Ticket income (net of refunds)", money(d.PnL.TicketIncome), false)
	kv("Sponsor income", money(d.PnL.SponsorIncome), false)
	kv("Vendor income", money(d.PnL.VendorIncome), false)
	kv("TOTAL INCOME", money(d.PnL.TotalIncome), true)
	kv("Hall cost", money(d.PnL.HallCost), false)
	kv("Crew cost", money(d.PnL.CrewCost), false)
	kv("Other expenses", money(d.PnL.Expenses), false)
	kv("TOTAL EXPENSES", money(d.PnL.TotalExpenses), true)
	kv("NET PROFIT / LOSS", money(d.PnL.NetPL), true)
	kv("Margin", pct(d.PnL.Margin), false)
	if d.PnL.RefundsPending > 0 {
		kv("Refund liability outstanding", money(d.PnL.RefundsPending), false)
	}

	if len(d.Cancellations) > 0 {
		block("Cancellation reasons")
		for _, c := range d.Cancellations {
			kv(c.Reason, fmt.Sprintf("%d booking(s), %s", c.Count, money(c.Amount)), false)
		}
	}

	// Sold and blocked seats only — the actionable rows.
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Sold, held and blocked seats", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 8)
	pdf.CellFormat(0, 5,
		"Available seats are omitted here; the CSV export lists every seat.", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	dw := []float64{28, 14, 16, 30, 22, 20, 34, 26}
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(242, 233, 251)
	for i, h := range []string{"Section", "Row", "Seat", "Tier", "Status", "Paid", "Booked by", "Ref"} {
		pdf.CellFormat(dw[i], 6, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Arial", "", 8)
	shown := 0
	for _, s := range d.Seats {
		if s.Status == OccAvailable {
			continue
		}
		paid := ""
		if s.Status == OccBooked {
			paid = money(s.PricePaid)
		}
		for i, c := range []string{s.Section, s.RowLabel, fmt.Sprintf("%d", s.SeatNumber),
			s.ZoneName, s.Status, paid, s.BuyerName, s.BookingRef} {
			pdf.CellFormat(dw[i], 5, opsTrunc(c, dw[i]), "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
		shown++
	}
	if shown == 0 {
		pdf.CellFormat(0, 6, "— no seats sold, held or blocked —", "1", 1, "L", false, 0, "")
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		`attachment; filename="stagex-seat-reservation-report.pdf"`)
	_ = pdf.Output(w)
}
