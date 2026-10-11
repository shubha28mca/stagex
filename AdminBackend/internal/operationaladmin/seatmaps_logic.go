package operationaladmin

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// This file holds the seat-map rules that involve no database: label
// generation, the row-block generator behind the designer's "add rows" tool,
// price resolution, validation and the summary the editor displays.
//
// Keeping them pure is deliberate — they are the parts with real logic, and
// they are unit-tested in seatmaps_logic_test.go without a Postgres.

// -----------------------------------------------------------------------------
// Wire types
//
// A save payload addresses tiers by `code` rather than by id. That lets the
// designer invent a brand-new tier and attach seats to it in a single request,
// and it means renaming or recolouring a tier never rewrites a seat row.
// -----------------------------------------------------------------------------

// ZoneInput is one price tier in a save payload.
type ZoneInput struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Color        string  `json:"color"`
	Price        float64 `json:"price"`
	DisplayOrder int     `json:"displayOrder"`
}

// SeatInput is one seat in a save payload. Price is a pointer because nil
// carries meaning: "inherit this seat's zone price". A non-nil value is a
// deliberate per-seat override.
type SeatInput struct {
	Section    string   `json:"section"`
	RowLabel   string   `json:"rowLabel"`
	RowIndex   int      `json:"rowIndex"`
	SeatNumber int      `json:"seatNumber"`
	SeatLabel  string   `json:"seatLabel"`
	ZoneCode   string   `json:"zoneCode"`
	Price      *float64 `json:"price"`
	Kind       string   `json:"kind"`
	PosX       int      `json:"posX"`
	PosY       int      `json:"posY"`
	IsActive   bool     `json:"isActive"`
}

// ElementInput is a non-seat canvas item: the stage, backstage, an aisle, a
// label. Purely presentational, so these are replaced wholesale on save.
type ElementInput struct {
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	PosX     int    `json:"posX"`
	PosY     int    `json:"posY"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Rotation int    `json:"rotation"`
	Color    string `json:"color"`
}

// SaveRequest is the whole arrangement, sent by the designer's Save button.
// The endpoint is a full replace (with id-stable seat upserts), which keeps the
// visual editor honest: what you see is exactly what is stored.
type SaveRequest struct {
	Name         string         `json:"name"`
	CanvasWidth  int            `json:"canvasWidth"`
	CanvasHeight int            `json:"canvasHeight"`
	Status       string         `json:"status"`
	Zones        []ZoneInput    `json:"zones"`
	Seats        []SeatInput    `json:"seats"`
	Elements     []ElementInput `json:"elements"`
}

// Valid seat kinds and element kinds — mirrored by CHECK constraints in
// migration 007, and validated here so a bad payload fails as a clean 400
// rather than a Postgres constraint violation surfacing as a 500.
var (
	seatKinds    = map[string]bool{"seat": true, "wheelchair": true, "companion": true, "house": true}
	elementKinds = map[string]bool{
		"stage": true, "backstage": true, "aisle": true,
		"entrance": true, "exit": true, "label": true, "screen": true,
	}
)

// sellable reports whether a seat can ever be sold to a buyer. House seats are
// held back, and inactive seats are blocked (broken, or equipment stands there)
// but stay in the map so the geometry survives.
func (s SeatInput) sellable() bool { return s.IsActive && s.Kind != "house" }

// -----------------------------------------------------------------------------
// Row labels
// -----------------------------------------------------------------------------

// rowLetters is the venue convention: I and O are skipped because they read as
// 1 and 0 on a printed ticket.
const rowLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"

// RowLabel returns the label for the i-th row (0-based) in venue lettering:
// A..H, J..N, P..Z, then AA, AB, … for very deep houses.
func RowLabel(i int) string {
	if i < 0 {
		return ""
	}
	n := len(rowLetters)
	if i < n {
		return string(rowLetters[i])
	}
	// Second cycle onwards: AA, AB, ... BA, BB, ...
	return string(rowLetters[(i/n)-1]) + string(rowLetters[i%n])
}

// SeatLabel is the human-readable seat name printed on a ticket ("C14").
func SeatLabel(rowLabel string, seatNumber int) string {
	return fmt.Sprintf("%s%d", rowLabel, seatNumber)
}

// -----------------------------------------------------------------------------
// Row-block generator — the designer's "add rows" tool
// -----------------------------------------------------------------------------

// BlockSpec describes a rectangular (optionally curved) block of rows to lay
// out. It is what the editor sends when Ops fills in "8 rows of 24, starting at
// row A, 3 rows from the stage".
type BlockSpec struct {
	Section string `json:"section"`
	Rows    int    `json:"rows"`
	// SeatsPerRow is the seat count in every generated row.
	SeatsPerRow int `json:"seatsPerRow"`
	// FirstRowIndex is the distance-from-stage of the first generated row, so a
	// balcony block can start at 13 while its lettering restarts.
	FirstRowIndex int `json:"firstRowIndex"`
	// FirstRowLetter offsets the label sequence; 0 == "A", 8 == "J".
	FirstRowLetter int    `json:"firstRowLetter"`
	NumberFrom     int    `json:"numberFrom"`
	ZoneCode       string `json:"zoneCode"`
	Kind           string `json:"kind"`
	// CenterX is the horizontal centre of the block; rows are centred on it so
	// rows of differing widths stay visually aligned.
	CenterX   int `json:"centerX"`
	OriginY   int `json:"originY"`
	SeatPitch int `json:"seatPitch"`
	RowPitch  int `json:"rowPitch"`
	// Curve lifts the ends of each row by this many units, giving the gentle arc
	// real auditoria have. 0 draws straight rows.
	Curve int `json:"curve"`
}

// GenerateBlock lays out a block of rows as seats. It is pure geometry — the
// caller merges the result into the map and saves.
func GenerateBlock(spec BlockSpec) ([]SeatInput, error) {
	if strings.TrimSpace(spec.Section) == "" {
		return nil, httpx.ErrBadRequest("section is required")
	}
	if spec.Rows < 1 || spec.Rows > 100 {
		return nil, httpx.ErrBadRequest("rows must be between 1 and 100")
	}
	if spec.SeatsPerRow < 1 || spec.SeatsPerRow > 200 {
		return nil, httpx.ErrBadRequest("seatsPerRow must be between 1 and 200")
	}
	if spec.Kind == "" {
		spec.Kind = "seat"
	}
	if !seatKinds[spec.Kind] {
		return nil, httpx.ErrBadRequest("unknown seat kind: " + spec.Kind)
	}
	if spec.SeatPitch <= 0 {
		spec.SeatPitch = 26
	}
	if spec.RowPitch <= 0 {
		spec.RowPitch = 30
	}
	if spec.NumberFrom < 1 {
		spec.NumberFrom = 1
	}

	// Centre each row on CenterX.
	span := (spec.SeatsPerRow - 1) * spec.SeatPitch
	startX := spec.CenterX - span/2

	// mid is the distance from centre to an end seat, used to normalise the arc
	// so Curve is exactly the lift at the outermost seat.
	mid := float64(spec.SeatsPerRow-1) / 2

	out := make([]SeatInput, 0, spec.Rows*spec.SeatsPerRow)
	for r := 0; r < spec.Rows; r++ {
		label := RowLabel(spec.FirstRowLetter + r)
		rowY := spec.OriginY + r*spec.RowPitch
		for c := 0; c < spec.SeatsPerRow; c++ {
			lift := 0
			if spec.Curve != 0 && mid > 0 {
				norm := (float64(c) - mid) / mid // -1 .. +1
				lift = int(math.Round(float64(spec.Curve) * norm * norm))
			}
			num := spec.NumberFrom + c
			out = append(out, SeatInput{
				Section:    spec.Section,
				RowLabel:   label,
				RowIndex:   spec.FirstRowIndex + r,
				SeatNumber: num,
				SeatLabel:  SeatLabel(label, num),
				ZoneCode:   spec.ZoneCode,
				Kind:       spec.Kind,
				PosX:       startX + c*spec.SeatPitch,
				PosY:       rowY - lift,
				IsActive:   true,
			})
		}
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Pricing
// -----------------------------------------------------------------------------

// EffectivePrice resolves what a seat costs, applying the single rule the
// database view v_hall_seat_pricing also encodes:
//
//	COALESCE(seat.price, zone.price, 0)
func EffectivePrice(seat SeatInput, zones map[string]ZoneInput) float64 {
	if seat.Price != nil {
		return *seat.Price
	}
	if z, ok := zones[seat.ZoneCode]; ok {
		return z.Price
	}
	return 0
}

// zoneIndex keys the tiers of a payload by code for lookup.
func zoneIndex(zones []ZoneInput) map[string]ZoneInput {
	m := make(map[string]ZoneInput, len(zones))
	for _, z := range zones {
		m[z.Code] = z
	}
	return m
}

// ZoneSummary is the per-tier rollup shown in the designer's sidebar.
type ZoneSummary struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Color   string  `json:"color"`
	Price   float64 `json:"price"`
	Seats   int     `json:"seats"`
	Revenue float64 `json:"revenue"`
}

// Summary is the at-a-glance state of an arrangement: how many seats are
// sellable, how many still lack a price, and what a full house is worth.
type Summary struct {
	TotalSeats       int           `json:"totalSeats"`
	BlockedSeats     int           `json:"blockedSeats"`
	HouseSeats       int           `json:"houseSeats"`
	UnpricedSeats    int           `json:"unpricedSeats"`
	OverriddenSeats  int           `json:"overriddenSeats"`
	PotentialRevenue float64       `json:"potentialRevenue"`
	Rows             int           `json:"rows"`
	Sections         []string      `json:"sections"`
	ByZone           []ZoneSummary `json:"byZone"`
}

// Summarize rolls up a payload. Only sellable seats contribute to the seat
// count and to revenue — counting blocked or house seats as income would
// overstate what the hall can actually take.
func Summarize(req SaveRequest) Summary {
	zones := zoneIndex(req.Zones)
	s := Summary{}

	perZone := map[string]*ZoneSummary{}
	for _, z := range req.Zones {
		perZone[z.Code] = &ZoneSummary{Code: z.Code, Name: z.Name, Color: z.Color, Price: z.Price}
	}

	sections := map[string]bool{}
	rows := map[string]bool{}

	for _, seat := range req.Seats {
		sections[seat.Section] = true
		rows[seat.Section+"|"+seat.RowLabel] = true

		if seat.Price != nil {
			s.OverriddenSeats++
		}
		if !seat.IsActive {
			s.BlockedSeats++
		}
		if seat.Kind == "house" {
			s.HouseSeats++
		}
		// One definition of "can this ever be sold", shared with the publish
		// gate — blocked and house seats must never inflate capacity or revenue.
		if !seat.sellable() {
			continue
		}

		price := EffectivePrice(seat, zones)
		if price <= 0 {
			s.UnpricedSeats++
		}
		s.TotalSeats++
		s.PotentialRevenue += price

		if zs, ok := perZone[seat.ZoneCode]; ok {
			zs.Seats++
			zs.Revenue += price
		}
	}

	s.Rows = len(rows)
	for name := range sections {
		s.Sections = append(s.Sections, name)
	}
	sort.Strings(s.Sections)

	for _, z := range req.Zones {
		if zs, ok := perZone[z.Code]; ok {
			s.ByZone = append(s.ByZone, *zs)
		}
	}
	return s
}

// -----------------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------------

// ValidateSaveRequest rejects a payload that could not be stored, or that would
// be unsafe to sell against. It returns the FIRST problem as a 400 so the
// designer can surface a precise message next to the Save button.
func ValidateSaveRequest(req *SaveRequest) error {
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Status != "draft" && req.Status != "published" {
		return httpx.ErrBadRequest(`status must be "draft" or "published"`)
	}
	if req.Name == "" {
		req.Name = "Main seating"
	}
	if req.CanvasWidth < 200 || req.CanvasWidth > 5000 ||
		req.CanvasHeight < 200 || req.CanvasHeight > 5000 {
		return httpx.ErrBadRequest("canvas dimensions must be between 200 and 5000")
	}

	// --- tiers ---
	seenZone := map[string]bool{}
	for i := range req.Zones {
		z := &req.Zones[i]
		z.Code = strings.TrimSpace(z.Code)
		if z.Code == "" {
			return httpx.ErrBadRequest("every price tier needs a code")
		}
		if seenZone[z.Code] {
			return httpx.ErrBadRequest("duplicate price tier code: " + z.Code)
		}
		seenZone[z.Code] = true
		if z.Name == "" {
			z.Name = z.Code
		}
		if z.Color == "" {
			z.Color = "#6a1b9a"
		}
		if z.Price < 0 {
			return httpx.ErrBadRequest("tier " + z.Code + " has a negative price")
		}
	}

	// --- seats ---
	seenSeat := map[string]bool{}
	for i := range req.Seats {
		s := &req.Seats[i]
		s.Section = strings.TrimSpace(s.Section)
		if s.Section == "" {
			s.Section = "Main"
		}
		s.RowLabel = strings.TrimSpace(s.RowLabel)
		if s.RowLabel == "" {
			return httpx.ErrBadRequest("every seat needs a row label")
		}
		if s.SeatNumber < 1 {
			return httpx.ErrBadRequest(
				fmt.Sprintf("seat number must be 1 or greater (row %s)", s.RowLabel))
		}
		if s.RowIndex < 0 {
			return httpx.ErrBadRequest(
				fmt.Sprintf("row %s has a negative distance from the stage", s.RowLabel))
		}
		if s.Kind == "" {
			s.Kind = "seat"
		}
		if !seatKinds[s.Kind] {
			return httpx.ErrBadRequest("unknown seat kind: " + s.Kind)
		}
		if s.Price != nil && *s.Price < 0 {
			return httpx.ErrBadRequest("seat " + s.SeatLabel + " has a negative price")
		}
		if s.ZoneCode != "" && !seenZone[s.ZoneCode] {
			return httpx.ErrBadRequest(
				"seat " + s.SeatLabel + " references unknown price tier " + s.ZoneCode)
		}
		if s.SeatLabel == "" {
			s.SeatLabel = SeatLabel(s.RowLabel, s.SeatNumber)
		}

		// The natural key, and the same one the upsert uses — a duplicate here
		// would silently collapse two seats into one row on save.
		key := s.Section + "|" + s.RowLabel + "|" + fmt.Sprint(s.SeatNumber)
		if seenSeat[key] {
			return httpx.ErrBadRequest(
				fmt.Sprintf("duplicate seat: %s row %s seat %d", s.Section, s.RowLabel, s.SeatNumber))
		}
		seenSeat[key] = true
	}

	// --- elements ---
	for i := range req.Elements {
		e := &req.Elements[i]
		if !elementKinds[e.Kind] {
			return httpx.ErrBadRequest("unknown element kind: " + e.Kind)
		}
		if e.Width < 1 || e.Height < 1 {
			return httpx.ErrBadRequest("element " + e.Kind + " must have a positive size")
		}
		if e.Color == "" {
			e.Color = "#0e1237"
		}
	}

	// --- publishing gate ---
	// A published map is what a buyer will be shown, so it must be complete:
	// at least one sellable seat, and no sellable seat without a price.
	if req.Status == "published" {
		sum := Summarize(*req)
		if sum.TotalSeats == 0 {
			return httpx.ErrBadRequest("cannot publish an arrangement with no sellable seats")
		}
		if sum.UnpricedSeats > 0 {
			return httpx.ErrBadRequest(fmt.Sprintf(
				"cannot publish: %d seat(s) have no price — assign a tier or a per-seat price first",
				sum.UnpricedSeats))
		}
	}
	return nil
}
