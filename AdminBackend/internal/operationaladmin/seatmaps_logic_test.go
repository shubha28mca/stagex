package operationaladmin

import (
	"errors"
	"testing"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// These tests cover the seat-map rules that carry real logic — venue row
// lettering, the row-block generator behind the designer's "add rows" tool,
// price resolution, the summary, and every validation gate. They need no
// database, so `go test ./...` exercises them anywhere.

func price(v float64) *float64 { return &v }

// badRequest asserts an error is a 400 APIError, and returns its message so a
// test can also check the wording the designer will show next to Save.
func badRequest(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var apiErr *httpx.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *httpx.APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 400 {
		t.Fatalf("expected status 400, got %d (%s)", apiErr.Status, apiErr.Message)
	}
	return apiErr.Message
}

// -----------------------------------------------------------------------------
// Row lettering
// -----------------------------------------------------------------------------

func TestRowLabelSkipsIAndO(t *testing.T) {
	// Venues skip I and O because they read as 1 and 0 on a printed ticket.
	cases := map[int]string{
		0: "A", 7: "H", 8: "J", 12: "N", 13: "P", 23: "Z",
		24: "AA", 25: "AB", 47: "AZ", 48: "BA",
	}
	for i, want := range cases {
		if got := RowLabel(i); got != want {
			t.Errorf("RowLabel(%d) = %q, want %q", i, got, want)
		}
	}
	for i := 0; i < 200; i++ {
		for _, bad := range []string{"I", "O"} {
			if RowLabel(i) == bad {
				t.Errorf("RowLabel(%d) produced the forbidden label %q", i, bad)
			}
		}
	}
	if got := RowLabel(-1); got != "" {
		t.Errorf("RowLabel(-1) = %q, want empty", got)
	}
}

func TestSeatLabel(t *testing.T) {
	if got := SeatLabel("C", 14); got != "C14" {
		t.Errorf("SeatLabel = %q, want C14", got)
	}
}

// -----------------------------------------------------------------------------
// Row-block generator
// -----------------------------------------------------------------------------

func TestGenerateBlockLaysOutACentredGrid(t *testing.T) {
	seats, err := GenerateBlock(BlockSpec{
		Section: "Orchestra", Rows: 3, SeatsPerRow: 5,
		FirstRowIndex: 0, FirstRowLetter: 0,
		CenterX: 600, OriginY: 200, SeatPitch: 20, RowPitch: 30,
		ZoneCode: "vip",
	})
	if err != nil {
		t.Fatalf("GenerateBlock: %v", err)
	}
	if len(seats) != 15 {
		t.Fatalf("got %d seats, want 15", len(seats))
	}

	first := seats[0]
	if first.RowLabel != "A" || first.SeatNumber != 1 || first.SeatLabel != "A1" {
		t.Errorf("first seat = %+v, want row A seat 1", first)
	}
	if first.ZoneCode != "vip" || first.Kind != "seat" || !first.IsActive {
		t.Errorf("first seat defaults wrong: %+v", first)
	}
	// 5 seats at pitch 20 spans 80; centred on 600 means seat 1 sits at 560.
	if first.PosX != 560 {
		t.Errorf("first seat PosX = %d, want 560 (centred on 600)", first.PosX)
	}
	if last := seats[4]; last.PosX != 640 {
		t.Errorf("last seat of row PosX = %d, want 640", last.PosX)
	}

	// row_index must be the distance from the stage, incrementing per row.
	for r := 0; r < 3; r++ {
		s := seats[r*5]
		if s.RowIndex != r {
			t.Errorf("row %d has RowIndex %d, want %d", r, s.RowIndex, r)
		}
		if wantY := 200 + r*30; s.PosY != wantY {
			t.Errorf("row %d PosY = %d, want %d", r, s.PosY, wantY)
		}
	}
}

func TestGenerateBlockHonoursOffsetsAndCurve(t *testing.T) {
	// A balcony block: lettering restarts at J (index 8) but it is physically
	// the 13th row back, and seat numbering starts at 101.
	seats, err := GenerateBlock(BlockSpec{
		Section: "Balcony", Rows: 1, SeatsPerRow: 5,
		FirstRowIndex: 13, FirstRowLetter: 8, NumberFrom: 101,
		CenterX: 100, OriginY: 500, SeatPitch: 10, RowPitch: 10,
		Curve: 8,
	})
	if err != nil {
		t.Fatalf("GenerateBlock: %v", err)
	}
	if seats[0].RowLabel != "J" {
		t.Errorf("FirstRowLetter 8 gave %q, want J", seats[0].RowLabel)
	}
	if seats[0].RowIndex != 13 {
		t.Errorf("RowIndex = %d, want 13", seats[0].RowIndex)
	}
	if seats[0].SeatNumber != 101 || seats[0].SeatLabel != "J101" {
		t.Errorf("numbering wrong: %+v", seats[0])
	}
	// Curve lifts the ends by exactly Curve and leaves the centre seat alone.
	if seats[0].PosY != 500-8 {
		t.Errorf("end seat PosY = %d, want %d (lifted by curve)", seats[0].PosY, 500-8)
	}
	if mid := seats[2]; mid.PosY != 500 {
		t.Errorf("centre seat PosY = %d, want 500 (unlifted)", mid.PosY)
	}
	if seats[4].PosY != 500-8 {
		t.Errorf("other end PosY = %d, want %d", seats[4].PosY, 500-8)
	}
}

func TestGenerateBlockRejectsBadSpecs(t *testing.T) {
	cases := []struct {
		name string
		spec BlockSpec
	}{
		{"no section", BlockSpec{Rows: 1, SeatsPerRow: 1}},
		{"zero rows", BlockSpec{Section: "A", Rows: 0, SeatsPerRow: 1}},
		{"too many rows", BlockSpec{Section: "A", Rows: 101, SeatsPerRow: 1}},
		{"zero seats", BlockSpec{Section: "A", Rows: 1, SeatsPerRow: 0}},
		{"too many seats", BlockSpec{Section: "A", Rows: 1, SeatsPerRow: 201}},
		{"bad kind", BlockSpec{Section: "A", Rows: 1, SeatsPerRow: 1, Kind: "sofa"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := GenerateBlock(c.spec)
			badRequest(t, err)
		})
	}
}

func TestGenerateBlockAppliesPitchDefaults(t *testing.T) {
	// A spec with no spacing must still produce a usable layout rather than
	// stacking every seat at the same point.
	seats, err := GenerateBlock(BlockSpec{Section: "Main", Rows: 2, SeatsPerRow: 2})
	if err != nil {
		t.Fatalf("GenerateBlock: %v", err)
	}
	if seats[0].PosX == seats[1].PosX {
		t.Error("seats in a row share an x position — seat pitch default not applied")
	}
	if seats[0].PosY == seats[2].PosY {
		t.Error("rows share a y position — row pitch default not applied")
	}
}

// -----------------------------------------------------------------------------
// Price resolution
// -----------------------------------------------------------------------------

func TestEffectivePriceInheritsZoneUnlessOverridden(t *testing.T) {
	zones := zoneIndex([]ZoneInput{{Code: "vip", Price: 2500}})

	inherit := SeatInput{ZoneCode: "vip"}
	if got := EffectivePrice(inherit, zones); got != 2500 {
		t.Errorf("inherited price = %v, want 2500", got)
	}

	override := SeatInput{ZoneCode: "vip", Price: price(1800)}
	if got := EffectivePrice(override, zones); got != 1800 {
		t.Errorf("overridden price = %v, want 1800", got)
	}

	// A zero override is a real decision (a comp seat), not "unset".
	free := SeatInput{ZoneCode: "vip", Price: price(0)}
	if got := EffectivePrice(free, zones); got != 0 {
		t.Errorf("zero override = %v, want 0 — a zero price must beat the tier", got)
	}

	orphan := SeatInput{ZoneCode: "gone"}
	if got := EffectivePrice(orphan, zones); got != 0 {
		t.Errorf("seat with an unknown tier = %v, want 0", got)
	}
}

// -----------------------------------------------------------------------------
// Summary
// -----------------------------------------------------------------------------

func TestSummarizeCountsOnlySellableSeats(t *testing.T) {
	req := SaveRequest{
		Zones: []ZoneInput{
			{Code: "vip", Name: "VIP", Price: 1000},
			{Code: "back", Name: "Back", Price: 100},
		},
		Seats: []SeatInput{
			{Section: "S", RowLabel: "A", SeatNumber: 1, ZoneCode: "vip", Kind: "seat", IsActive: true},
			{Section: "S", RowLabel: "A", SeatNumber: 2, ZoneCode: "vip", Kind: "seat", IsActive: true, Price: price(250)},
			// blocked: geometry kept, never sold
			{Section: "S", RowLabel: "A", SeatNumber: 3, ZoneCode: "vip", Kind: "seat", IsActive: false},
			// held for the house: active but not sellable
			{Section: "S", RowLabel: "B", SeatNumber: 1, ZoneCode: "back", Kind: "house", IsActive: true},
			// no tier and no override -> unpriced
			{Section: "S", RowLabel: "B", SeatNumber: 2, Kind: "seat", IsActive: true},
		},
	}
	s := Summarize(req)

	if s.TotalSeats != 3 {
		t.Errorf("TotalSeats = %d, want 3 (2 vip + 1 unpriced; blocked and house excluded)", s.TotalSeats)
	}
	if s.BlockedSeats != 1 {
		t.Errorf("BlockedSeats = %d, want 1", s.BlockedSeats)
	}
	if s.HouseSeats != 1 {
		t.Errorf("HouseSeats = %d, want 1", s.HouseSeats)
	}
	if s.UnpricedSeats != 1 {
		t.Errorf("UnpricedSeats = %d, want 1", s.UnpricedSeats)
	}
	if s.OverriddenSeats != 1 {
		t.Errorf("OverriddenSeats = %d, want 1", s.OverriddenSeats)
	}
	// 1000 (inherited) + 250 (override) + 0 (unpriced). The blocked VIP seat and
	// the house seat must not contribute.
	if s.PotentialRevenue != 1250 {
		t.Errorf("PotentialRevenue = %v, want 1250", s.PotentialRevenue)
	}
	if s.Rows != 2 {
		t.Errorf("Rows = %d, want 2", s.Rows)
	}
	if len(s.Sections) != 1 || s.Sections[0] != "S" {
		t.Errorf("Sections = %v, want [S]", s.Sections)
	}

	byZone := map[string]ZoneSummary{}
	for _, z := range s.ByZone {
		byZone[z.Code] = z
	}
	if z := byZone["vip"]; z.Seats != 2 || z.Revenue != 1250 {
		t.Errorf("vip rollup = %d seats / %v, want 2 / 1250", z.Seats, z.Revenue)
	}
	if z := byZone["back"]; z.Seats != 0 {
		t.Errorf("back rollup = %d seats, want 0 (its only seat is a house seat)", z.Seats)
	}
}

// -----------------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------------

// validMap is a minimal arrangement that passes validation, for tests that want
// to break exactly one thing.
func validMap() SaveRequest {
	return SaveRequest{
		Name: "Main", CanvasWidth: 1200, CanvasHeight: 800, Status: "draft",
		Zones: []ZoneInput{{Code: "vip", Name: "VIP", Color: "#000", Price: 500}},
		Seats: []SeatInput{
			{Section: "S", RowLabel: "A", SeatNumber: 1, ZoneCode: "vip", Kind: "seat", IsActive: true},
		},
		Elements: []ElementInput{{Kind: "stage", Width: 100, Height: 50}},
	}
}

func TestValidateFillsDefaults(t *testing.T) {
	req := SaveRequest{
		CanvasWidth: 1200, CanvasHeight: 800,
		Zones:    []ZoneInput{{Code: "vip", Price: 10}},
		Seats:    []SeatInput{{RowLabel: "A", SeatNumber: 7, ZoneCode: "vip", IsActive: true}},
		Elements: []ElementInput{{Kind: "stage", Width: 10, Height: 10}},
	}
	if err := ValidateSaveRequest(&req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Status != "draft" {
		t.Errorf("Status = %q, want draft", req.Status)
	}
	if req.Name != "Main seating" {
		t.Errorf("Name = %q, want the default", req.Name)
	}
	if req.Zones[0].Name != "vip" {
		t.Errorf("zone Name = %q, want to fall back to the code", req.Zones[0].Name)
	}
	if req.Zones[0].Color == "" {
		t.Error("zone Color was left empty")
	}
	if req.Seats[0].Section != "Main" {
		t.Errorf("seat Section = %q, want Main", req.Seats[0].Section)
	}
	if req.Seats[0].Kind != "seat" {
		t.Errorf("seat Kind = %q, want seat", req.Seats[0].Kind)
	}
	// The label a ticket prints must be derived when the editor omits it.
	if req.Seats[0].SeatLabel != "A7" {
		t.Errorf("SeatLabel = %q, want A7", req.Seats[0].SeatLabel)
	}
	if req.Elements[0].Color == "" {
		t.Error("element Color was left empty")
	}
}

func TestValidateRejectsDuplicateSeat(t *testing.T) {
	// (section, row, number) is the upsert key — a duplicate would silently
	// collapse two seats into one row on save.
	req := validMap()
	req.Seats = append(req.Seats, SeatInput{
		Section: "S", RowLabel: "A", SeatNumber: 1, ZoneCode: "vip", Kind: "seat", IsActive: true,
	})
	if msg := badRequest(t, ValidateSaveRequest(&req)); msg == "" {
		t.Error("expected a message naming the duplicate")
	}
}

func TestValidateAllowsSameSeatNumberInDifferentSections(t *testing.T) {
	req := validMap()
	req.Seats = append(req.Seats, SeatInput{
		Section: "Balcony", RowLabel: "A", SeatNumber: 1, ZoneCode: "vip", Kind: "seat", IsActive: true,
	})
	if err := ValidateSaveRequest(&req); err != nil {
		t.Fatalf("same row/number in another section must be allowed: %v", err)
	}
}

func TestValidateRejectsBadPayloads(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SaveRequest)
	}{
		{"bad status", func(r *SaveRequest) { r.Status = "live" }},
		{"canvas too small", func(r *SaveRequest) { r.CanvasWidth = 10 }},
		{"canvas too large", func(r *SaveRequest) { r.CanvasHeight = 99999 }},
		{"blank zone code", func(r *SaveRequest) { r.Zones[0].Code = "  " }},
		{"duplicate zone code", func(r *SaveRequest) {
			r.Zones = append(r.Zones, ZoneInput{Code: "vip", Price: 1})
		}},
		{"negative zone price", func(r *SaveRequest) { r.Zones[0].Price = -1 }},
		{"blank row label", func(r *SaveRequest) { r.Seats[0].RowLabel = "" }},
		{"seat number zero", func(r *SaveRequest) { r.Seats[0].SeatNumber = 0 }},
		{"negative row index", func(r *SaveRequest) { r.Seats[0].RowIndex = -1 }},
		{"bad seat kind", func(r *SaveRequest) { r.Seats[0].Kind = "beanbag" }},
		{"negative seat price", func(r *SaveRequest) { r.Seats[0].Price = price(-5) }},
		{"unknown tier reference", func(r *SaveRequest) { r.Seats[0].ZoneCode = "nope" }},
		{"bad element kind", func(r *SaveRequest) { r.Elements[0].Kind = "sofa" }},
		{"zero-size element", func(r *SaveRequest) { r.Elements[0].Width = 0 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := validMap()
			c.mutate(&req)
			badRequest(t, ValidateSaveRequest(&req))
		})
	}
}

func TestPublishRequiresEverySellableSeatToBePriced(t *testing.T) {
	// A published map is what a buyer sees, so an unpriced sellable seat must
	// block publishing rather than quietly selling for nothing.
	req := validMap()
	req.Status = "published"
	req.Seats = append(req.Seats, SeatInput{
		Section: "S", RowLabel: "B", SeatNumber: 1, Kind: "seat", IsActive: true, // no tier, no price
	})
	msg := badRequest(t, ValidateSaveRequest(&req))
	if msg == "" {
		t.Error("expected a message explaining the unpriced seats")
	}

	// Blocked and house seats are never sold, so they must NOT block publishing.
	ok := validMap()
	ok.Status = "published"
	ok.Seats = append(ok.Seats,
		SeatInput{Section: "S", RowLabel: "B", SeatNumber: 1, Kind: "seat", IsActive: false},
		SeatInput{Section: "S", RowLabel: "B", SeatNumber: 2, Kind: "house", IsActive: true},
	)
	if err := ValidateSaveRequest(&ok); err != nil {
		t.Errorf("blocked/house seats must not block publishing: %v", err)
	}
}

func TestPublishRequiresAtLeastOneSellableSeat(t *testing.T) {
	req := validMap()
	req.Status = "published"
	req.Seats = nil
	badRequest(t, ValidateSaveRequest(&req))
}

func TestDraftMayBeIncomplete(t *testing.T) {
	// Ops must be able to save work in progress — an unpriced seat is fine in a
	// draft, it just cannot be published.
	req := validMap()
	req.Status = "draft"
	req.Seats = append(req.Seats, SeatInput{
		Section: "S", RowLabel: "B", SeatNumber: 1, Kind: "seat", IsActive: true,
	})
	if err := ValidateSaveRequest(&req); err != nil {
		t.Errorf("a draft with unpriced seats must save: %v", err)
	}
}

func TestValidateAcceptsAnEmptyDraft(t *testing.T) {
	// Creating an arrangement then saving before adding anything must work.
	req := SaveRequest{CanvasWidth: 1200, CanvasHeight: 800}
	if err := ValidateSaveRequest(&req); err != nil {
		t.Errorf("empty draft rejected: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Round trip
// -----------------------------------------------------------------------------

func TestGeneratedBlockValidatesAndSummarizes(t *testing.T) {
	// The generator's output must be directly saveable and publishable — this is
	// the exact path the designer takes: add rows, assign a tier, publish.
	seats, err := GenerateBlock(BlockSpec{
		Section: "Orchestra", Rows: 8, SeatsPerRow: 24,
		CenterX: 600, OriginY: 170, ZoneCode: "premium", Curve: 6,
	})
	if err != nil {
		t.Fatalf("GenerateBlock: %v", err)
	}
	req := SaveRequest{
		CanvasWidth: 1200, CanvasHeight: 800, Status: "published",
		Zones:    []ZoneInput{{Code: "premium", Name: "Premium", Price: 1500}},
		Seats:    seats,
		Elements: []ElementInput{{Kind: "stage", Width: 400, Height: 80}},
	}
	if err := ValidateSaveRequest(&req); err != nil {
		t.Fatalf("generated block failed validation: %v", err)
	}
	s := Summarize(req)
	if s.TotalSeats != 192 {
		t.Errorf("TotalSeats = %d, want 192", s.TotalSeats)
	}
	if s.PotentialRevenue != 192*1500 {
		t.Errorf("PotentialRevenue = %v, want %v", s.PotentialRevenue, 192*1500)
	}
	if s.Rows != 8 {
		t.Errorf("Rows = %d, want 8", s.Rows)
	}
	if s.UnpricedSeats != 0 {
		t.Errorf("UnpricedSeats = %d, want 0", s.UnpricedSeats)
	}
}
