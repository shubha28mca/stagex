package operationaladmin

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Hall seating arrangements — Operational Admin only.
//
// A hall has at most one arrangement. Ops designs it in the visual editor and
// saves the whole thing with a single PUT: the designer is the source of truth
// for geometry, so a full replace keeps "what you see" and "what is stored"
// identical. Seats are upserted on their natural key
// (section, row_label, seat_number), which keeps admin_hall_seats.id stable
// across redesigns so a future booking row never points at a deleted seat.
//
// The rules with real logic (layout generation, price resolution, validation)
// live in seatmaps_logic.go and are unit-tested without a database.

// -----------------------------------------------------------------------------
// Read models
// -----------------------------------------------------------------------------

// Zone is a stored price tier.
type Zone struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Color        string  `json:"color"`
	Price        float64 `json:"price"`
	DisplayOrder int     `json:"displayOrder"`
}

// Seat is a stored seat. EffectivePrice is the resolved
// COALESCE(seat.price, zone.price, 0) so the client never repeats the rule.
type Seat struct {
	ID             string   `json:"id"`
	Section        string   `json:"section"`
	RowLabel       string   `json:"rowLabel"`
	RowIndex       int      `json:"rowIndex"`
	SeatNumber     int      `json:"seatNumber"`
	SeatLabel      string   `json:"seatLabel"`
	ZoneCode       string   `json:"zoneCode"`
	Price          *float64 `json:"price"`
	EffectivePrice float64  `json:"effectivePrice"`
	Kind           string   `json:"kind"`
	PosX           int      `json:"posX"`
	PosY           int      `json:"posY"`
	IsActive       bool     `json:"isActive"`
}

// Element is a stored non-seat canvas item (stage, backstage, aisle, label).
type Element struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	PosX     int    `json:"posX"`
	PosY     int    `json:"posY"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Rotation int    `json:"rotation"`
	Color    string `json:"color"`
}

// SeatMap is a hall's complete arrangement, as the designer loads it.
type SeatMap struct {
	ID           string    `json:"id"`
	HallID       string    `json:"hallId"`
	HallName     string    `json:"hallName"`
	HallCity     string    `json:"hallCity"`
	HallCapacity int       `json:"hallCapacity"`
	Name         string    `json:"name"`
	CanvasWidth  int       `json:"canvasWidth"`
	CanvasHeight int       `json:"canvasHeight"`
	Status       string    `json:"status"`
	Version      int       `json:"version"`
	TotalSeats   int       `json:"totalSeats"`
	Zones        []Zone    `json:"zones"`
	Seats        []Seat    `json:"seats"`
	Elements     []Element `json:"elements"`
	Summary      Summary   `json:"summary"`
}

// HallSeatMapRow is one line of the index: every hall, and the state of its
// arrangement if it has one. Drives the "Seat Maps" landing page.
type HallSeatMapRow struct {
	HallID     string `json:"hallId"`
	HallName   string `json:"hallName"`
	City       string `json:"city"`
	Capacity   int    `json:"capacity"`
	HasMap     bool   `json:"hasMap"`
	SeatMapID  string `json:"seatMapId"`
	MapName    string `json:"mapName"`
	Status     string `json:"status"`
	TotalSeats int    `json:"totalSeats"`
	Zones      int    `json:"zones"`
}

// BulkSeatUpdate assigns a tier and/or a price to a set of already-saved seats
// — the "select seats, set a price" action in the designer.
type BulkSeatUpdate struct {
	SeatIDs []string `json:"seatIds"`
	// nil leaves the tier alone; "" clears it.
	ZoneCode *string `json:"zoneCode"`
	// nil leaves the per-seat price alone.
	Price *float64 `json:"price"`
	// true resets the seats to inherit their tier price.
	ClearPrice bool `json:"clearPrice"`
	// nil leaves it alone; used to block/unblock a selection.
	IsActive *bool `json:"isActive"`
}

type seatMapService struct{ pool *pgxpool.Pool }

// -----------------------------------------------------------------------------
// Reads
// -----------------------------------------------------------------------------

func (s seatMapService) index(ctx context.Context) ([]HallSeatMapRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT h.id, h.name, h.city, h.capacity,
		       m.id IS NOT NULL,
		       COALESCE(m.id::text, ''), COALESCE(m.name, ''),
		       COALESCE(m.status, ''), COALESCE(m.total_seats, 0),
		       (SELECT count(*) FROM admin_hall_seat_zones z WHERE z.seat_map_id = m.id)
		FROM admin_halls h
		LEFT JOIN admin_hall_seat_maps m ON m.hall_id = h.id
		ORDER BY h.city, h.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HallSeatMapRow{}
	for rows.Next() {
		var r HallSeatMapRow
		if err := rows.Scan(&r.HallID, &r.HallName, &r.City, &r.Capacity,
			&r.HasMap, &r.SeatMapID, &r.MapName, &r.Status, &r.TotalSeats, &r.Zones); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// get loads a hall's arrangement. It distinguishes "no such hall" (404 hall)
// from "hall has no arrangement yet" (404 seat map) so the UI can offer to
// create one rather than showing a dead end.
func (s seatMapService) get(ctx context.Context, hallID string) (*SeatMap, error) {
	var m SeatMap
	err := s.pool.QueryRow(ctx, `
		SELECT m.id, m.hall_id, h.name, h.city, h.capacity,
		       m.name, m.canvas_width, m.canvas_height, m.status, m.version, m.total_seats
		FROM admin_hall_seat_maps m
		JOIN admin_halls h ON h.id = m.hall_id
		WHERE m.hall_id = $1`, hallID).
		Scan(&m.ID, &m.HallID, &m.HallName, &m.HallCity, &m.HallCapacity,
			&m.Name, &m.CanvasWidth, &m.CanvasHeight, &m.Status, &m.Version, &m.TotalSeats)
	if errors.Is(err, pgx.ErrNoRows) {
		// Tell the two cases apart for a better message upstream.
		var hallExists bool
		if e := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM admin_halls WHERE id=$1)`, hallID).Scan(&hallExists); e != nil {
			return nil, e
		}
		if !hallExists {
			return nil, httpx.ErrNotFound("hall not found")
		}
		return nil, httpx.ErrNotFound("this hall has no seating arrangement yet")
	}
	if err != nil {
		return nil, err
	}

	if m.Zones, err = s.loadZones(ctx, m.ID); err != nil {
		return nil, err
	}
	if m.Seats, err = s.loadSeats(ctx, m.ID); err != nil {
		return nil, err
	}
	if m.Elements, err = s.loadElements(ctx, m.ID); err != nil {
		return nil, err
	}
	m.Summary = Summarize(toSaveRequest(m))
	return &m, nil
}

func (s seatMapService) loadZones(ctx context.Context, mapID string) ([]Zone, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, code, name, color, price, display_order
		FROM admin_hall_seat_zones WHERE seat_map_id=$1
		ORDER BY display_order, code`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Zone{}
	for rows.Next() {
		var z Zone
		if err := rows.Scan(&z.ID, &z.Code, &z.Name, &z.Color, &z.Price, &z.DisplayOrder); err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

func (s seatMapService) loadSeats(ctx context.Context, mapID string) ([]Seat, error) {
	// Ordering front-to-back then left-to-right is what the editor and any
	// client seat picker both want, so it is done here once.
	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.section, s.row_label, s.row_index, s.seat_number, s.seat_label,
		       COALESCE(z.code, ''), s.price, COALESCE(s.price, z.price, 0),
		       s.kind, s.pos_x, s.pos_y, s.is_active
		FROM admin_hall_seats s
		LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id
		WHERE s.seat_map_id = $1
		ORDER BY s.section, s.row_index, s.seat_number`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Seat{}
	for rows.Next() {
		var st Seat
		if err := rows.Scan(&st.ID, &st.Section, &st.RowLabel, &st.RowIndex, &st.SeatNumber,
			&st.SeatLabel, &st.ZoneCode, &st.Price, &st.EffectivePrice,
			&st.Kind, &st.PosX, &st.PosY, &st.IsActive); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s seatMapService) loadElements(ctx context.Context, mapID string) ([]Element, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, label, pos_x, pos_y, width, height, rotation, color
		FROM admin_hall_seat_elements WHERE seat_map_id=$1 ORDER BY created_at`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Element{}
	for rows.Next() {
		var e Element
		if err := rows.Scan(&e.ID, &e.Kind, &e.Label, &e.PosX, &e.PosY,
			&e.Width, &e.Height, &e.Rotation, &e.Color); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// toSaveRequest re-shapes a loaded map into the payload form so Summarize (and
// the publish gate) run over exactly the same structure whether the data came
// from the database or from the designer.
func toSaveRequest(m SeatMap) SaveRequest {
	req := SaveRequest{
		Name: m.Name, CanvasWidth: m.CanvasWidth, CanvasHeight: m.CanvasHeight, Status: m.Status,
	}
	for _, z := range m.Zones {
		req.Zones = append(req.Zones, ZoneInput{
			Code: z.Code, Name: z.Name, Color: z.Color, Price: z.Price, DisplayOrder: z.DisplayOrder,
		})
	}
	for _, s := range m.Seats {
		req.Seats = append(req.Seats, SeatInput{
			Section: s.Section, RowLabel: s.RowLabel, RowIndex: s.RowIndex,
			SeatNumber: s.SeatNumber, SeatLabel: s.SeatLabel, ZoneCode: s.ZoneCode,
			Price: s.Price, Kind: s.Kind, PosX: s.PosX, PosY: s.PosY, IsActive: s.IsActive,
		})
	}
	for _, e := range m.Elements {
		req.Elements = append(req.Elements, ElementInput{
			Kind: e.Kind, Label: e.Label, PosX: e.PosX, PosY: e.PosY,
			Width: e.Width, Height: e.Height, Rotation: e.Rotation, Color: e.Color,
		})
	}
	return req
}

// -----------------------------------------------------------------------------
// Writes
// -----------------------------------------------------------------------------

func (s seatMapService) hallExists(ctx context.Context, hallID string) error {
	var ok bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM admin_halls WHERE id=$1)`, hallID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return httpx.ErrNotFound("hall not found")
	}
	return nil
}

// create starts an empty draft arrangement for a hall, pre-seeded with a stage
// and a backstage block so the designer opens onto an oriented canvas rather
// than a blank one — Ops always needs both, and their position is what every
// row_index is measured from.
func (s seatMapService) create(ctx context.Context, hallID string) (*SeatMap, error) {
	if err := s.hallExists(ctx, hallID); err != nil {
		return nil, err
	}
	var mapID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_hall_seat_maps (hall_id, name)
		VALUES ($1, 'Main seating')
		ON CONFLICT (hall_id) DO NOTHING
		RETURNING id`, hallID).Scan(&mapID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrConflict("this hall already has a seating arrangement")
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO admin_hall_seat_elements (seat_map_id, kind, label, pos_x, pos_y, width, height, color)
		VALUES ($1,'backstage','BACKSTAGE',450,20,300,40,'#171c4f'),
		       ($1,'stage','STAGE',400,70,400,80,'#0e1237')`, mapID); err != nil {
		return nil, err
	}
	return s.get(ctx, hallID)
}

// save writes the whole arrangement in one transaction. It creates the map if
// the hall has none, so the designer's Save works whether or not create ran.
func (s seatMapService) save(ctx context.Context, hallID string, req SaveRequest) (*SeatMap, error) {
	if err := ValidateSaveRequest(&req); err != nil {
		return nil, err
	}
	if err := s.hallExists(ctx, hallID); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	// Rollback is a no-op once Commit succeeded; this guarantees a failed save
	// never leaves a half-written map.
	defer func() { _ = tx.Rollback(ctx) }()

	var mapID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO admin_hall_seat_maps (hall_id, name, canvas_width, canvas_height, status)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (hall_id) DO UPDATE SET
			name = EXCLUDED.name,
			canvas_width = EXCLUDED.canvas_width,
			canvas_height = EXCLUDED.canvas_height,
			status = EXCLUDED.status,
			version = admin_hall_seat_maps.version + 1,
			updated_at = now()
		RETURNING id`,
		hallID, req.Name, req.CanvasWidth, req.CanvasHeight, req.Status).Scan(&mapID); err != nil {
		return nil, err
	}

	// --- tiers: upsert by code, then drop the ones the designer removed ------
	zCodes := make([]string, len(req.Zones))
	if len(req.Zones) > 0 {
		zNames := make([]string, len(req.Zones))
		zColors := make([]string, len(req.Zones))
		zPrices := make([]float64, len(req.Zones))
		zOrders := make([]int32, len(req.Zones))
		for i, z := range req.Zones {
			zCodes[i], zNames[i], zColors[i] = z.Code, z.Name, z.Color
			zPrices[i], zOrders[i] = z.Price, int32(z.DisplayOrder)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_hall_seat_zones (seat_map_id, code, name, color, price, display_order)
			SELECT $1, k.code, k.name, k.color, k.price, k.display_order
			FROM unnest($2::text[],$3::text[],$4::text[],$5::numeric[],$6::int[])
			     AS k(code, name, color, price, display_order)
			ON CONFLICT (seat_map_id, code) DO UPDATE SET
				name = EXCLUDED.name, color = EXCLUDED.color, price = EXCLUDED.price,
				display_order = EXCLUDED.display_order, updated_at = now()`,
			mapID, zCodes, zNames, zColors, zPrices, zOrders); err != nil {
			return nil, err
		}
	}
	// Seats in a dropped tier survive via ON DELETE SET NULL and are then
	// reported as unpriced, which is what blocks publishing.
	if _, err := tx.Exec(ctx,
		`DELETE FROM admin_hall_seat_zones WHERE seat_map_id=$1 AND code <> ALL($2::text[])`,
		mapID, zCodes); err != nil {
		return nil, err
	}

	// --- seats: batch upsert on the natural key, keeping ids stable ---------
	n := len(req.Seats)
	secs := make([]string, n)
	rowLabels := make([]string, n)
	rowIdx := make([]int32, n)
	seatNums := make([]int32, n)
	labels := make([]string, n)
	prices := make([]float64, n)
	hasPrice := make([]bool, n)
	kinds := make([]string, n)
	xs := make([]int32, n)
	ys := make([]int32, n)
	actives := make([]bool, n)
	zoneCodes := make([]string, n)
	for i, st := range req.Seats {
		secs[i], rowLabels[i] = st.Section, st.RowLabel
		rowIdx[i], seatNums[i] = int32(st.RowIndex), int32(st.SeatNumber)
		labels[i], kinds[i], zoneCodes[i] = st.SeatLabel, st.Kind, st.ZoneCode
		xs[i], ys[i], actives[i] = int32(st.PosX), int32(st.PosY), st.IsActive
		if st.Price != nil {
			prices[i], hasPrice[i] = *st.Price, true
		}
	}
	if n > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_hall_seats
				(seat_map_id, zone_id, section, row_label, row_index, seat_number, seat_label,
				 price, kind, pos_x, pos_y, is_active)
			SELECT $1, z.id, k.section, k.row_label, k.row_index, k.seat_number, k.seat_label,
			       CASE WHEN k.has_price THEN k.price ELSE NULL END,
			       k.kind, k.pos_x, k.pos_y, k.is_active
			FROM unnest($2::text[],$3::text[],$4::int[],$5::int[],$6::text[],$7::numeric[],
			            $8::bool[],$9::text[],$10::int[],$11::int[],$12::bool[],$13::text[])
			     AS k(section, row_label, row_index, seat_number, seat_label, price,
			          has_price, kind, pos_x, pos_y, is_active, zone_code)
			LEFT JOIN admin_hall_seat_zones z ON z.seat_map_id = $1 AND z.code = k.zone_code
			ON CONFLICT (seat_map_id, section, row_label, seat_number) DO UPDATE SET
				zone_id = EXCLUDED.zone_id, row_index = EXCLUDED.row_index,
				seat_label = EXCLUDED.seat_label, price = EXCLUDED.price,
				kind = EXCLUDED.kind, pos_x = EXCLUDED.pos_x, pos_y = EXCLUDED.pos_y,
				is_active = EXCLUDED.is_active, updated_at = now()`,
			mapID, secs, rowLabels, rowIdx, seatNums, labels, prices,
			hasPrice, kinds, xs, ys, actives, zoneCodes); err != nil {
			return nil, err
		}
	}
	// Remove seats the designer deleted. With empty arrays this correctly
	// clears every seat (unnest of empty arrays yields no rows).
	if _, err := tx.Exec(ctx, `
		DELETE FROM admin_hall_seats s
		WHERE s.seat_map_id = $1
		  AND NOT EXISTS (
		      SELECT 1 FROM unnest($2::text[],$3::text[],$4::int[])
		               AS k(section, row_label, seat_number)
		      WHERE k.section = s.section
		        AND k.row_label = s.row_label
		        AND k.seat_number = s.seat_number
		  )`, mapID, secs, rowLabels, seatNums); err != nil {
		return nil, err
	}

	// --- elements: nothing references them, so replace wholesale ------------
	if _, err := tx.Exec(ctx,
		`DELETE FROM admin_hall_seat_elements WHERE seat_map_id=$1`, mapID); err != nil {
		return nil, err
	}
	if len(req.Elements) > 0 {
		eKinds := make([]string, len(req.Elements))
		eLabels := make([]string, len(req.Elements))
		eX := make([]int32, len(req.Elements))
		eY := make([]int32, len(req.Elements))
		eW := make([]int32, len(req.Elements))
		eH := make([]int32, len(req.Elements))
		eRot := make([]int32, len(req.Elements))
		eColor := make([]string, len(req.Elements))
		for i, e := range req.Elements {
			eKinds[i], eLabels[i], eColor[i] = e.Kind, e.Label, e.Color
			eX[i], eY[i] = int32(e.PosX), int32(e.PosY)
			eW[i], eH[i], eRot[i] = int32(e.Width), int32(e.Height), int32(e.Rotation)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_hall_seat_elements
				(seat_map_id, kind, label, pos_x, pos_y, width, height, rotation, color)
			SELECT $1, k.kind, k.label, k.pos_x, k.pos_y, k.width, k.height, k.rotation, k.color
			FROM unnest($2::text[],$3::text[],$4::int[],$5::int[],$6::int[],$7::int[],$8::int[],$9::text[])
			     AS k(kind, label, pos_x, pos_y, width, height, rotation, color)`,
			mapID, eKinds, eLabels, eX, eY, eW, eH, eRot, eColor); err != nil {
			return nil, err
		}
	}

	// Keep the denormalized count in step with the seats just written. Same
	// expression as the seed migration, so the two can never disagree.
	if _, err := tx.Exec(ctx, `
		UPDATE admin_hall_seat_maps SET total_seats = (
			SELECT count(*) FROM admin_hall_seats
			WHERE seat_map_id = $1 AND is_active AND kind <> 'house'
		) WHERE id = $1`, mapID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.get(ctx, hallID)
}

// setStatus flips an arrangement between draft and published. Publishing runs
// the same completeness gate as a save, so a map can never become sellable
// while seats are unpriced.
func (s seatMapService) setStatus(ctx context.Context, hallID, status string) (*SeatMap, error) {
	if status != "draft" && status != "published" {
		return nil, httpx.ErrBadRequest(`status must be "draft" or "published"`)
	}
	current, err := s.get(ctx, hallID)
	if err != nil {
		return nil, err
	}
	if status == "published" {
		req := toSaveRequest(*current)
		req.Status = "published"
		if err := ValidateSaveRequest(&req); err != nil {
			return nil, err
		}
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE admin_hall_seat_maps
		SET status=$2, version=version+1, updated_at=now()
		WHERE hall_id=$1`, hallID, status); err != nil {
		return nil, err
	}
	return s.get(ctx, hallID)
}

// bulkUpdate applies a tier, a price, or a blocked flag to a selection of
// seats. This is the "select one or many seats → set the price" action; it
// touches only the fields the caller actually supplied.
func (s seatMapService) bulkUpdate(ctx context.Context, hallID string, req BulkSeatUpdate) (*SeatMap, error) {
	if len(req.SeatIDs) == 0 {
		return nil, httpx.ErrBadRequest("select at least one seat")
	}
	if req.Price != nil && *req.Price < 0 {
		return nil, httpx.ErrBadRequest("price cannot be negative")
	}
	if req.Price != nil && req.ClearPrice {
		return nil, httpx.ErrBadRequest("cannot set a price and clear it in the same request")
	}

	var mapID string
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM admin_hall_seat_maps WHERE hall_id=$1`, hallID).Scan(&mapID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound("this hall has no seating arrangement yet")
		}
		return nil, err
	}

	// Resolve the tier code once, and reject an unknown one rather than
	// silently clearing the tier on every selected seat.
	var zoneID *string
	if req.ZoneCode != nil && *req.ZoneCode != "" {
		var id string
		if err := s.pool.QueryRow(ctx,
			`SELECT id FROM admin_hall_seat_zones WHERE seat_map_id=$1 AND code=$2`,
			mapID, *req.ZoneCode).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, httpx.ErrBadRequest("unknown price tier: " + *req.ZoneCode)
			}
			return nil, err
		}
		zoneID = &id
	}

	// Each SET is guarded by a flag so an omitted field is left untouched.
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_hall_seats SET
			zone_id   = CASE WHEN $3 THEN $4::uuid    ELSE zone_id   END,
			price     = CASE WHEN $5 THEN $6::numeric
			                 WHEN $7 THEN NULL        ELSE price     END,
			is_active = CASE WHEN $8 THEN $9::boolean ELSE is_active END,
			updated_at = now()
		WHERE seat_map_id = $1 AND id = ANY($2::uuid[])`,
		mapID, req.SeatIDs,
		req.ZoneCode != nil, zoneID,
		req.Price != nil, req.Price,
		req.ClearPrice,
		req.IsActive != nil, req.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("none of those seats belong to this hall's arrangement")
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE admin_hall_seat_maps SET total_seats = (
			SELECT count(*) FROM admin_hall_seats
			WHERE seat_map_id = $1 AND is_active AND kind <> 'house'
		), version = version + 1, updated_at = now() WHERE id = $1`, mapID); err != nil {
		return nil, err
	}
	return s.get(ctx, hallID)
}

func (s seatMapService) delete(ctx context.Context, hallID string) error {
	// Cascades to zones, seats and elements via the FKs in migration 007.
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_hall_seat_maps WHERE hall_id=$1`, hallID)
	return err
}

// -----------------------------------------------------------------------------
// Routes — all behind the Ops role guard
// -----------------------------------------------------------------------------

func registerSeatMaps(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := seatMapService{pool}

	mux.Handle("GET /admin/ops/seat-maps", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			data, err := svc.index(r.Context())
			respond(w, data, err)
		})))

	mux.Handle("GET /admin/ops/halls/{hallId}/seat-map", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			data, err := svc.get(r.Context(), r.PathValue("hallId"))
			respond(w, data, err)
		})))

	mux.Handle("POST /admin/ops/halls/{hallId}/seat-map", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			out, err := svc.create(r.Context(), r.PathValue("hallId"))
			respondCreated(w, out, err)
		})))

	// The designer's Save — a full replace of the arrangement.
	mux.Handle("PUT /admin/ops/halls/{hallId}/seat-map", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var req SaveRequest
			if err := httpx.Decode(r, &req); err != nil {
				httpx.Error(w, err)
				return
			}
			out, err := svc.save(r.Context(), r.PathValue("hallId"), req)
			respond(w, out, err)
		})))

	mux.Handle("DELETE /admin/ops/halls/{hallId}/seat-map", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			respond(w, map[string]bool{"deleted": true},
				svc.delete(r.Context(), r.PathValue("hallId")))
		})))

	mux.Handle("POST /admin/ops/halls/{hallId}/seat-map/publish", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			out, err := svc.setStatus(r.Context(), r.PathValue("hallId"), "published")
			respond(w, out, err)
		})))

	mux.Handle("POST /admin/ops/halls/{hallId}/seat-map/unpublish", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			out, err := svc.setStatus(r.Context(), r.PathValue("hallId"), "draft")
			respond(w, out, err)
		})))

	// Assign a tier / price / blocked flag to a selection of saved seats.
	mux.Handle("PATCH /admin/ops/halls/{hallId}/seat-map/seats", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var req BulkSeatUpdate
			if err := httpx.Decode(r, &req); err != nil {
				httpx.Error(w, err)
				return
			}
			out, err := svc.bulkUpdate(r.Context(), r.PathValue("hallId"), req)
			respond(w, out, err)
		})))

	// Lay out a block of rows WITHOUT saving. The designer inserts the result
	// into its canvas, so the server owns the geometry rule and the editor and
	// the stored map can never drift apart.
	mux.Handle("POST /admin/ops/seat-maps/generate-rows", guard(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var spec BlockSpec
			if err := httpx.Decode(r, &spec); err != nil {
				httpx.Error(w, err)
				return
			}
			seats, err := GenerateBlock(spec)
			respond(w, seats, err)
		})))
}
