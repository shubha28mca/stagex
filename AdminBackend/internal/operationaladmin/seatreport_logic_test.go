package operationaladmin

import (
	"testing"
	"time"
)

// Unit tests for the seat-reservation report rules: the occupancy verdict Ops
// sees per seat, the roll-up that answers "how many seats are available / paid
// for / blocked", and the ticket money reconciliation that feeds the event P&L.
//
// No database and no wall clock — every rule takes `now` as an argument.

var reportNow = time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

const testGrace = 5 * time.Minute

// -----------------------------------------------------------------------------
// Occupancy verdict
// -----------------------------------------------------------------------------

func TestOccupancyStatusBasicStates(t *testing.T) {
	cases := []struct {
		name  string
		facts SeatFacts
		want  string
	}{
		{"never touched", SeatFacts{IsActive: true, Kind: "seat"}, OccAvailable},
		{"explicitly available", SeatFacts{IsActive: true, Kind: "seat", ESSStatus: "available"}, OccAvailable},
		{"sold", SeatFacts{IsActive: true, Kind: "seat", ESSStatus: "booked"}, OccBooked},
		{"live hold", SeatFacts{
			IsActive: true, Kind: "seat", ESSStatus: "held",
			HoldExpiresAt: reportNow.Add(5 * time.Minute)}, OccHeld},
		{"lapsed hold is available again", SeatFacts{
			IsActive: true, Kind: "seat", ESSStatus: "held",
			HoldExpiresAt: reportNow.Add(-time.Second)}, OccAvailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OccupancyStatus(c.facts, reportNow, testGrace); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestOccupancyStatusSurfacesBlockedAndHouseSeats(t *testing.T) {
	// These two are the whole reason the admin view exists: a buyer never sees
	// them, but Ops must be able to account for every seat in the hall.
	blocked := SeatFacts{IsActive: false, Kind: "seat"}
	if got := OccupancyStatus(blocked, reportNow, testGrace); got != OccBlocked {
		t.Errorf("blocked seat = %q, want %q", got, OccBlocked)
	}
	house := SeatFacts{IsActive: true, Kind: "house"}
	if got := OccupancyStatus(house, reportNow, testGrace); got != OccHouse {
		t.Errorf("house seat = %q, want %q", got, OccHouse)
	}
}

func TestOccupancyStatusLayoutDecisionOutranksSalesState(t *testing.T) {
	// A seat Ops has blocked must report as blocked even if it carries stale
	// hold or booking state — it is not on sale, so nothing else matters.
	stale := SeatFacts{
		IsActive: false, Kind: "seat", ESSStatus: "booked",
		HoldExpiresAt: reportNow.Add(time.Hour),
	}
	if got := OccupancyStatus(stale, reportNow, testGrace); got != OccBlocked {
		t.Errorf("got %q, want %q — a blocked seat outranks its sales state", got, OccBlocked)
	}

	houseHeld := SeatFacts{
		IsActive: true, Kind: "house", ESSStatus: "held",
		HoldExpiresAt: reportNow.Add(time.Hour),
	}
	if got := OccupancyStatus(houseHeld, reportNow, testGrace); got != OccHouse {
		t.Errorf("got %q, want %q", got, OccHouse)
	}
}

func TestOccupancyStatusAppliesThePaymentGrace(t *testing.T) {
	// Mirrors the participant API: a hold backed by a payment in flight must
	// still read as taken past its deadline, or Ops would see a seat as free
	// while a buyer is being charged for it.
	f := SeatFacts{
		IsActive: true, Kind: "seat", ESSStatus: "held",
		HoldExpiresAt: reportNow, PaymentInFlight: true,
	}
	if got := OccupancyStatus(f, reportNow.Add(time.Minute), testGrace); got != OccHeld {
		t.Errorf("during grace: got %q, want %q", got, OccHeld)
	}
	if got := OccupancyStatus(f, reportNow.Add(testGrace), testGrace); got != OccAvailable {
		t.Errorf("after grace: got %q, want %q", got, OccAvailable)
	}

	noPayment := f
	noPayment.PaymentInFlight = false
	if got := OccupancyStatus(noPayment, reportNow.Add(time.Minute), testGrace); got != OccAvailable {
		t.Errorf("no payment in flight: got %q, want %q", got, OccAvailable)
	}
}

// -----------------------------------------------------------------------------
// Summary roll-up
// -----------------------------------------------------------------------------

// hall builds a small mixed hall: 2 sold, 1 held, 2 free, 1 blocked, 1 house.
func hall() ([]SeatOccupancy, []ZoneRevenue) {
	zones := []ZoneRevenue{
		{Code: "vip", Name: "VIP", Color: "#111"},
		{Code: "back", Name: "Back", Color: "#222"},
	}
	seats := []SeatOccupancy{
		{SeatLabel: "A1", ZoneCode: "vip", ListPrice: 1000, Status: OccBooked, PricePaid: 1000},
		{SeatLabel: "A2", ZoneCode: "vip", ListPrice: 1000, Status: OccBooked, PricePaid: 800}, // coupon
		{SeatLabel: "A3", ZoneCode: "vip", ListPrice: 1000, Status: OccHeld},
		{SeatLabel: "A4", ZoneCode: "vip", ListPrice: 1000, Status: OccAvailable},
		{SeatLabel: "B1", ZoneCode: "back", ListPrice: 200, Status: OccAvailable},
		{SeatLabel: "B2", ZoneCode: "back", ListPrice: 200, Status: OccBlocked},
		{SeatLabel: "B3", ZoneCode: "back", ListPrice: 200, Status: OccHouse},
	}
	return seats, zones
}

func TestSummarizeSeatsCounts(t *testing.T) {
	s, _ := SummarizeSeats(hall())

	if s.MappedSeats != 7 {
		t.Errorf("MappedSeats = %d, want 7", s.MappedSeats)
	}
	// Blocked and house seats are excluded from what can ever be sold.
	if s.Sellable != 5 {
		t.Errorf("Sellable = %d, want 5 (7 - 1 blocked - 1 house)", s.Sellable)
	}
	if s.Booked != 2 {
		t.Errorf("Booked = %d, want 2", s.Booked)
	}
	if s.Held != 1 {
		t.Errorf("Held = %d, want 1", s.Held)
	}
	if s.Available != 2 {
		t.Errorf("Available = %d, want 2", s.Available)
	}
	if s.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", s.Blocked)
	}
	if s.House != 1 {
		t.Errorf("House = %d, want 1", s.House)
	}
	// Every seat must land in exactly one bucket.
	if total := s.Booked + s.Held + s.Available + s.Blocked + s.House; total != s.MappedSeats {
		t.Errorf("buckets sum to %d but MappedSeats is %d — a seat was double-counted or lost",
			total, s.MappedSeats)
	}
}

func TestSummarizeSeatsMoney(t *testing.T) {
	s, _ := SummarizeSeats(hall())

	// The ceiling counts only sellable seats: 4 VIP at 1000 + 1 back at 200.
	if s.ListValue != 4200 {
		t.Errorf("ListValue = %v, want 4200 (blocked and house excluded)", s.ListValue)
	}
	// Realised is what was actually paid, not the list price.
	if s.Realised != 1800 {
		t.Errorf("Realised = %v, want 1800 (1000 + 800 after a coupon)", s.Realised)
	}
	if s.HeldValue != 1000 {
		t.Errorf("HeldValue = %v, want 1000", s.HeldValue)
	}
	if s.UnsoldValue != 1200 {
		t.Errorf("UnsoldValue = %v, want 1200 (1000 VIP + 200 back)", s.UnsoldValue)
	}
	// 2 of 5 sellable seats sold.
	if s.OccupancyPct != 40 {
		t.Errorf("OccupancyPct = %v, want 40", s.OccupancyPct)
	}
	// 1800 realised of a 4200 ceiling.
	if s.FillRatePct != 42.86 {
		t.Errorf("FillRatePct = %v, want 42.86", s.FillRatePct)
	}
}

func TestSummarizeSeatsPerTier(t *testing.T) {
	_, byZone := SummarizeSeats(hall())

	got := map[string]ZoneRevenue{}
	for _, z := range byZone {
		got[z.Code] = z
	}

	vip := got["vip"]
	if vip.Seats != 4 || vip.Booked != 2 || vip.Held != 1 || vip.Available != 1 {
		t.Errorf("vip rollup = %+v, want 4 seats / 2 booked / 1 held / 1 available", vip)
	}
	if vip.ListValue != 4000 || vip.Realised != 1800 {
		t.Errorf("vip money = list %v realised %v, want 4000 / 1800", vip.ListValue, vip.Realised)
	}

	back := got["back"]
	if back.Seats != 3 || back.Blocked != 1 || back.Available != 1 {
		t.Errorf("back rollup = %+v, want 3 seats / 1 blocked / 1 available", back)
	}
	// The blocked and house seats are counted in Seats but contribute no value.
	if back.ListValue != 200 {
		t.Errorf("back ListValue = %v, want 200 (only the one sellable seat)", back.ListValue)
	}
}

func TestSummarizeSeatsUntieredSeatsAreReported(t *testing.T) {
	// A seat with no price tier is an anomaly Ops needs to see, not something
	// to silently drop from the report.
	seats := []SeatOccupancy{
		{SeatLabel: "X1", ZoneCode: "", ListPrice: 0, Status: OccAvailable},
	}
	s, byZone := SummarizeSeats(seats, []ZoneRevenue{{Code: "vip", Name: "VIP"}})
	if s.MappedSeats != 1 || s.Sellable != 1 {
		t.Errorf("untiered seat was dropped: %+v", s)
	}
	if len(byZone) != 1 || byZone[0].Name != "No tier" {
		t.Errorf("expected a 'No tier' bucket, got %+v", byZone)
	}
}

func TestSummarizeSeatsEmptyHall(t *testing.T) {
	// No seats must not divide by zero.
	s, byZone := SummarizeSeats(nil, nil)
	if s.MappedSeats != 0 || s.OccupancyPct != 0 || s.FillRatePct != 0 {
		t.Errorf("empty hall produced %+v, want zeroes", s)
	}
	if len(byZone) != 0 {
		t.Errorf("empty hall produced tiers: %+v", byZone)
	}
}

func TestSummarizeSeatsFullHouse(t *testing.T) {
	seats := []SeatOccupancy{
		{ZoneCode: "a", ListPrice: 100, Status: OccBooked, PricePaid: 100},
		{ZoneCode: "a", ListPrice: 100, Status: OccBooked, PricePaid: 100},
	}
	s, _ := SummarizeSeats(seats, []ZoneRevenue{{Code: "a", Name: "A"}})
	if s.OccupancyPct != 100 {
		t.Errorf("OccupancyPct = %v, want 100", s.OccupancyPct)
	}
	if s.FillRatePct != 100 {
		t.Errorf("FillRatePct = %v, want 100", s.FillRatePct)
	}
	if s.UnsoldValue != 0 {
		t.Errorf("UnsoldValue = %v, want 0", s.UnsoldValue)
	}
}

func TestSummarizeSeatsAllBlockedHallIsNotUndersold(t *testing.T) {
	// A hall where everything is blocked has no sellable seats, so occupancy is
	// undefined rather than 0% — reporting 0% would imply lost sales.
	seats := []SeatOccupancy{
		{ZoneCode: "a", ListPrice: 100, Status: OccBlocked},
		{ZoneCode: "a", ListPrice: 100, Status: OccHouse},
	}
	s, _ := SummarizeSeats(seats, []ZoneRevenue{{Code: "a", Name: "A"}})
	if s.Sellable != 0 {
		t.Errorf("Sellable = %d, want 0", s.Sellable)
	}
	if s.ListValue != 0 {
		t.Errorf("ListValue = %v, want 0 — unsellable seats have no value", s.ListValue)
	}
	if s.OccupancyPct != 0 || s.FillRatePct != 0 {
		t.Errorf("percentages = %v / %v, want 0 with no division by zero",
			s.OccupancyPct, s.FillRatePct)
	}
}

// -----------------------------------------------------------------------------
// Ticket money
// -----------------------------------------------------------------------------

func TestComputeTicketFinanceNetsRefunds(t *testing.T) {
	// Gross is everything collected, including bookings later cancelled; the
	// refund is then subtracted, so the figures reconcile against the bank.
	got := ComputeTicketFinance(TicketFinance{
		Gross: 10000, Refunds: 2500, RefundsPending: 1500,
		TicketsSold: 8, TicketsCancelled: 2,
	})
	if got.Net != 7500 {
		t.Errorf("Net = %v, want 7500 (10000 gross - 2500 refunds)", got.Net)
	}
	if got.RefundsPending != 1500 {
		t.Errorf("RefundsPending = %v, want 1500", got.RefundsPending)
	}
	// The pending part is a subset of refunds, never additional to them.
	if got.RefundsPending > got.Refunds {
		t.Error("RefundsPending must never exceed Refunds")
	}
}

func TestComputeTicketFinanceNoSales(t *testing.T) {
	got := ComputeTicketFinance(TicketFinance{})
	if got.Gross != 0 || got.Net != 0 || got.Refunds != 0 {
		t.Errorf("no sales produced %+v, want zeroes", got)
	}
}

func TestComputeTicketFinanceFullyRefunded(t *testing.T) {
	// Every booking cancelled: the event kept nothing, and Net must be exactly
	// zero rather than a rounding artefact.
	got := ComputeTicketFinance(TicketFinance{Gross: 3333.33, Refunds: 3333.33})
	if got.Net != 0 {
		t.Errorf("Net = %v, want 0", got.Net)
	}
}

func TestComputeTicketFinanceRounds(t *testing.T) {
	got := ComputeTicketFinance(TicketFinance{Gross: 100.005, Refunds: 0.004})
	if got.Gross != 100.01 {
		t.Errorf("Gross = %v, want 100.01", got.Gross)
	}
	if got.Refunds != 0 {
		t.Errorf("Refunds = %v, want 0", got.Refunds)
	}
}

// -----------------------------------------------------------------------------
// P&L integration
// -----------------------------------------------------------------------------

func TestApplyTicketsToPnL(t *testing.T) {
	base := PnL{
		Revenue: 5000, SponsorIncome: 2000, VendorIncome: 1000,
		CrewCost: 3000, Expenses: 1000, HallCost: 45000,
	}
	tickets := ComputeTicketFinance(TicketFinance{Gross: 60000, Refunds: 5000, RefundsPending: 2000})

	p := ApplyTicketsToPnL(base, tickets)

	if p.TicketIncome != 55000 {
		t.Errorf("TicketIncome = %v, want 55000 (net of refunds)", p.TicketIncome)
	}
	if p.TicketGross != 60000 || p.TicketRefunds != 5000 {
		t.Errorf("gross/refund lines not carried through: %+v", p)
	}
	// Ticket income must be part of total income, or the P&L is wrong once a
	// hall sells reserved seating.
	if p.TotalIncome != 63000 {
		t.Errorf("TotalIncome = %v, want 63000 (5000 + 2000 + 1000 + 55000)", p.TotalIncome)
	}
	if p.TotalExpenses != 49000 {
		t.Errorf("TotalExpenses = %v, want 49000", p.TotalExpenses)
	}
	if p.NetPL != 14000 {
		t.Errorf("NetPL = %v, want 14000", p.NetPL)
	}
	if p.Margin != 22.22 {
		t.Errorf("Margin = %v, want 22.22", p.Margin)
	}
	// A pending refund is a liability, not an expense — it must not be
	// subtracted twice.
	if p.RefundsPending != 2000 {
		t.Errorf("RefundsPending = %v, want 2000", p.RefundsPending)
	}
}

func TestApplyTicketsToPnLProducesALossWhenCostsExceedIncome(t *testing.T) {
	// The report has to be able to say "this event lost money".
	base := PnL{Revenue: 1000, CrewCost: 5000, HallCost: 45000}
	p := ApplyTicketsToPnL(base, ComputeTicketFinance(TicketFinance{Gross: 10000}))

	if p.TotalIncome != 11000 {
		t.Errorf("TotalIncome = %v, want 11000", p.TotalIncome)
	}
	if p.NetPL != -39000 {
		t.Errorf("NetPL = %v, want -39000", p.NetPL)
	}
	if p.Margin >= 0 {
		t.Errorf("Margin = %v, want negative on a loss", p.Margin)
	}
}

func TestApplyTicketsToPnLNoIncomeDoesNotDivideByZero(t *testing.T) {
	p := ApplyTicketsToPnL(PnL{HallCost: 1000}, TicketFinance{})
	if p.TotalIncome != 0 {
		t.Errorf("TotalIncome = %v, want 0", p.TotalIncome)
	}
	if p.Margin != 0 {
		t.Errorf("Margin = %v, want 0 rather than NaN/Inf", p.Margin)
	}
	if p.NetPL != -1000 {
		t.Errorf("NetPL = %v, want -1000", p.NetPL)
	}
}

func TestApplyTicketsToPnLIsIdempotent(t *testing.T) {
	// The report calls pnl() which already folds tickets in; applying the same
	// figures again must not double-count them.
	base := PnL{Revenue: 1000}
	tf := ComputeTicketFinance(TicketFinance{Gross: 500})
	once := ApplyTicketsToPnL(base, tf)
	twice := ApplyTicketsToPnL(once, tf)
	if once.TotalIncome != twice.TotalIncome {
		t.Errorf("TotalIncome changed on re-apply: %v then %v", once.TotalIncome, twice.TotalIncome)
	}
	if twice.TotalIncome != 1500 {
		t.Errorf("TotalIncome = %v, want 1500", twice.TotalIncome)
	}
}

// -----------------------------------------------------------------------------
// Cancellation breakdown ordering
// -----------------------------------------------------------------------------

func TestSortReasonCountsIsStable(t *testing.T) {
	// Go randomises map iteration, so the report would otherwise reorder itself
	// between runs of the same data.
	in := []CancellationReasonCount{
		{Reason: "travel_issue", Count: 2, Amount: 200},
		{Reason: "change_of_plan", Count: 5, Amount: 500},
		{Reason: "other", Count: 2, Amount: 100},
	}
	got := SortReasonCounts(in)
	if got[0].Reason != "change_of_plan" {
		t.Errorf("expected the commonest reason first, got %q", got[0].Reason)
	}
	// Ties break alphabetically.
	if got[1].Reason != "other" || got[2].Reason != "travel_issue" {
		t.Errorf("ties not broken alphabetically: %q then %q", got[1].Reason, got[2].Reason)
	}
}
