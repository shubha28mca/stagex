package operationaladmin

import (
	"math"
	"sort"
	"time"
)

// Seat-occupancy and seat-revenue rules for the Ops seat-reservation report.
//
// Kept free of SQL and of the wall clock so they can be unit-tested: the
// occupancy verdict and the money reconciliation are the parts with real logic,
// and both are easy to get subtly wrong.

// Occupancy verdicts. The first two are ADMIN-ONLY — a buyer never sees a
// blocked or house seat at all, but Ops must, because those are exactly the
// seats they need to account for when arranging a hall.
const (
	OccAvailable = "available"
	OccHeld      = "held"
	OccBooked    = "booked"
	OccBlocked   = "blocked" // seat exists in the layout but is not sellable
	OccHouse     = "house"   // held back for the house / press
)

// SeatFacts is the raw state of one seat for one event, as stored.
type SeatFacts struct {
	// From the hall's arrangement.
	IsActive bool
	Kind     string
	// From event_seat_status; empty status means no row, i.e. never touched.
	ESSStatus       string
	HoldExpiresAt   time.Time
	PaymentInFlight bool
}

// OccupancyStatus resolves what Ops should see for one seat.
//
// Order matters. A seat that Ops blocked, or reserved for the house, is
// reported as such even if it somehow carries stale hold state — the layout
// decision outranks the sales state, because a blocked seat is not on sale in
// the first place.
//
// Expiry is judged by TIME (with the payment grace applied), exactly as the
// participant API judges it, so the two views can never disagree about whether
// a seat is actually taken.
func OccupancyStatus(f SeatFacts, now time.Time, grace time.Duration) string {
	if !f.IsActive {
		return OccBlocked
	}
	if f.Kind == "house" {
		return OccHouse
	}
	if f.ESSStatus == "booked" {
		return OccBooked
	}
	if f.ESSStatus == "held" {
		deadline := f.HoldExpiresAt
		if f.PaymentInFlight {
			deadline = deadline.Add(grace)
		}
		if now.Before(deadline) {
			return OccHeld
		}
	}
	return OccAvailable
}

// SeatSummary is the headline count Ops asked for: how many seats are
// available, paid for, being paid for, and blocked.
type SeatSummary struct {
	MappedSeats int `json:"mappedSeats"`
	// Sellable excludes blocked and house seats — the real denominator for
	// occupancy, and the number that should be compared against a hall's
	// registered capacity.
	Sellable  int `json:"sellable"`
	Booked    int `json:"booked"`
	Held      int `json:"held"`
	Available int `json:"available"`
	Blocked   int `json:"blocked"`
	House     int `json:"house"`

	// ListValue is a full house at the arrangement's prices — the ceiling.
	ListValue float64 `json:"listValue"`
	// Realised is what has actually been paid for the seats currently sold.
	Realised float64 `json:"realised"`
	// HeldValue is money in flight: seats claimed but not yet paid for.
	HeldValue float64 `json:"heldValue"`
	// UnsoldValue is what is still on the table.
	UnsoldValue float64 `json:"unsoldValue"`

	// OccupancyPct is booked / sellable, so blocked seats never flatter it.
	OccupancyPct float64 `json:"occupancyPct"`
	// FillRatePct is realised / list value — how much of the possible money
	// was actually taken. Differs from occupancy when cheap seats sell first.
	FillRatePct float64 `json:"fillRatePct"`
}

// ZoneRevenue is the per-tier rollup: which price bands are selling.
type ZoneRevenue struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	Seats     int     `json:"seats"`
	Booked    int     `json:"booked"`
	Held      int     `json:"held"`
	Available int     `json:"available"`
	Blocked   int     `json:"blocked"`
	ListValue float64 `json:"listValue"`
	Realised  float64 `json:"realised"`
}

// SeatOccupancy is one seat on the report: where it is, what it should cost,
// and — when sold — who bought it and what they actually paid.
type SeatOccupancy struct {
	SeatID     string `json:"seatId"`
	Section    string `json:"section"`
	RowLabel   string `json:"rowLabel"`
	RowIndex   int    `json:"rowIndex"`
	SeatNumber int    `json:"seatNumber"`
	SeatLabel  string `json:"seatLabel"`
	ZoneCode   string `json:"zoneCode"`
	ZoneName   string `json:"zoneName"`
	ZoneColor  string `json:"zoneColor"`
	Kind       string `json:"kind"`
	PosX       int    `json:"posX"`
	PosY       int    `json:"posY"`

	// ListPrice is what the arrangement says this seat costs.
	ListPrice float64 `json:"listPrice"`
	// Status is the resolved occupancy verdict.
	Status string `json:"status"`

	// --- populated only when the seat is booked ---
	BookingID  string     `json:"bookingId,omitempty"`
	BookingRef string     `json:"bookingRef,omitempty"`
	BuyerName  string     `json:"buyerName,omitempty"`
	BuyerPhone string     `json:"buyerPhone,omitempty"`
	PricePaid  float64    `json:"pricePaid,omitempty"`
	BookedAt   *time.Time `json:"bookedAt,omitempty"`

	// --- populated only when the seat is held ---
	HoldExpiresAt *time.Time `json:"holdExpiresAt,omitempty"`
	// PaymentInFlight distinguishes "someone is mid-payment" from "someone is
	// still deciding", which is what Ops needs when a hall is nearly full.
	PaymentInFlight bool `json:"paymentInFlight,omitempty"`
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// SummarizeSeats rolls the seat list up into the headline counts and the
// per-tier breakdown.
//
// Blocked and house seats contribute to MappedSeats but NOT to Sellable,
// ListValue or occupancy — counting a seat nobody can buy as unsold inventory
// would make every hall look permanently under-sold.
func SummarizeSeats(seats []SeatOccupancy, zones []ZoneRevenue) (SeatSummary, []ZoneRevenue) {
	s := SeatSummary{}
	byZone := map[string]*ZoneRevenue{}
	order := make([]string, 0, len(zones))
	for i := range zones {
		z := zones[i]
		byZone[z.Code] = &ZoneRevenue{Code: z.Code, Name: z.Name, Color: z.Color}
		order = append(order, z.Code)
	}
	// Seats with no tier still need somewhere to land on the report.
	const untiered = ""
	if _, ok := byZone[untiered]; !ok {
		byZone[untiered] = &ZoneRevenue{Code: untiered, Name: "No tier", Color: "#c9c4d8"}
	}

	for _, seat := range seats {
		s.MappedSeats++
		z := byZone[seat.ZoneCode]
		if z == nil {
			z = byZone[untiered]
		}
		z.Seats++

		switch seat.Status {
		case OccBlocked:
			s.Blocked++
			z.Blocked++
			continue // not sellable: no value attributed
		case OccHouse:
			s.House++
			continue
		}

		// From here on the seat is sellable, so it counts toward the ceiling.
		s.Sellable++
		s.ListValue += seat.ListPrice
		z.ListValue += seat.ListPrice

		switch seat.Status {
		case OccBooked:
			s.Booked++
			z.Booked++
			// What they actually paid, which can differ from the list price if
			// a coupon applied or Ops re-priced the seat after the sale.
			s.Realised += seat.PricePaid
			z.Realised += seat.PricePaid
		case OccHeld:
			s.Held++
			z.Held++
			s.HeldValue += seat.ListPrice
		default:
			s.Available++
			z.Available++
			s.UnsoldValue += seat.ListPrice
		}
	}

	if s.Sellable > 0 {
		s.OccupancyPct = round2(float64(s.Booked) / float64(s.Sellable) * 100)
	}
	if s.ListValue > 0 {
		s.FillRatePct = round2(s.Realised / s.ListValue * 100)
	}
	s.ListValue = round2(s.ListValue)
	s.Realised = round2(s.Realised)
	s.HeldValue = round2(s.HeldValue)
	s.UnsoldValue = round2(s.UnsoldValue)

	out := make([]ZoneRevenue, 0, len(byZone))
	for _, code := range order {
		if z := byZone[code]; z != nil && z.Seats > 0 {
			z.ListValue = round2(z.ListValue)
			z.Realised = round2(z.Realised)
			out = append(out, *z)
		}
	}
	// Untiered seats last — they are an anomaly, not a tier.
	if z := byZone[untiered]; z.Seats > 0 {
		z.ListValue = round2(z.ListValue)
		z.Realised = round2(z.Realised)
		out = append(out, *z)
	}
	return s, out
}

// TicketFinance is the ticket-sales side of an event's P&L.
//
// It is stated as gross-minus-refunds rather than just "confirmed revenue" so
// the numbers reconcile against the bank: money was actually received for
// bookings that were later cancelled, and it leaves again as a refund.
type TicketFinance struct {
	// Gross is every rupee successfully collected for tickets, including
	// bookings later cancelled.
	Gross float64 `json:"gross"`
	// Refunds is what cancelling gave back (settled or still owed).
	Refunds float64 `json:"refunds"`
	// Net is what the event actually keeps: Gross - Refunds.
	Net float64 `json:"net"`
	// RefundsPending is the part of Refunds not yet paid out — an outstanding
	// liability, and the finance team's work queue.
	RefundsPending float64 `json:"refundsPending"`

	TicketsSold       int `json:"ticketsSold"`
	TicketsCancelled  int `json:"ticketsCancelled"`
	Bookings          int `json:"bookings"`
	CancelledBookings int `json:"cancelledBookings"`
}

// ComputeTicketFinance derives the net figure and rounds the money.
func ComputeTicketFinance(f TicketFinance) TicketFinance {
	f.Gross = round2(f.Gross)
	f.Refunds = round2(f.Refunds)
	f.RefundsPending = round2(f.RefundsPending)
	f.Net = round2(f.Gross - f.Refunds)
	return f
}

// CancellationReasonCount is one line of "why are people cancelling?".
type CancellationReasonCount struct {
	Reason string  `json:"reason"`
	Count  int     `json:"count"`
	Amount float64 `json:"amount"`
}

// SortReasonCounts orders the breakdown by count, then reason, so the report
// is stable between runs (map iteration in Go is randomised).
func SortReasonCounts(in []CancellationReasonCount) []CancellationReasonCount {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Count != in[j].Count {
			return in[i].Count > in[j].Count
		}
		return in[i].Reason < in[j].Reason
	})
	return in
}

// ApplyTicketsToPnL folds ticket sales into an event's P&L.
//
// Ticket money is NET of refunds, so a cancelled booking neither inflates
// income nor gets counted twice. Registration revenue, sponsor and vendor
// income are untouched — this only adds the ticket line and recomputes the
// totals, so the existing P&L stays comparable.
func ApplyTicketsToPnL(p PnL, t TicketFinance) PnL {
	p.TicketGross = round2(t.Gross)
	p.TicketRefunds = round2(t.Refunds)
	p.TicketIncome = round2(t.Net)
	p.RefundsPending = round2(t.RefundsPending)

	p.TotalIncome = round2(p.Revenue + p.SponsorIncome + p.VendorIncome + p.TicketIncome)
	p.TotalExpenses = round2(p.CrewCost + p.Expenses + p.HallCost)
	p.NetPL = round2(p.TotalIncome - p.TotalExpenses)
	if p.TotalIncome > 0 {
		p.Margin = round2(p.NetPL / p.TotalIncome * 100)
	} else {
		p.Margin = 0
	}
	return p
}
