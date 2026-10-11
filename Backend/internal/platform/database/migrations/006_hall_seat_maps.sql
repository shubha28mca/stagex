-- =============================================================================
-- Migration 006 — Hall seating arrangements (Operational Admin owns the data)
--
-- A hall has AT MOST ONE seating arrangement (a "seat map"). Operational Admin
-- designs it visually — drag seats, place the stage and backstage, group seats
-- into price tiers — and it then becomes the geometry + pricing reference for
-- every event held in that hall.
--
-- Four tables, deliberately normalized so the participant (client) side can
-- render the same map and resolve a price per seat in one join:
--
--   admin_hall_seat_maps            one row per hall — the canvas
--     ├─ admin_hall_seat_zones      price tiers (VIP / Premium / Balcony …)
--     ├─ admin_hall_seats           one row per physical seat
--     └─ admin_hall_seat_elements   non-seat furniture (stage, backstage, aisle)
--
-- PRICE RESOLUTION is a single rule, applied in exactly one place:
--
--     effective price = COALESCE(seat.price, zone.price, 0)
--
-- i.e. a seat inherits the price of its zone unless Ops set a per-seat
-- override. The view `v_hall_seat_pricing` at the bottom exposes that already
-- resolved, so client code never re-implements the rule.
--
-- SEAT IDENTITY is (seat_map_id, section, row_label, seat_number) — a natural
-- key. Saving the map upserts on that key, so `admin_hall_seats.id` is STABLE
-- across redesigns. That matters because the per-event booking tables
-- (migration 008) reference seat ids; re-saving a map must not orphan sold
-- tickets.
--
-- OWNERSHIP: these are `admin_`-prefixed master tables written only by the
-- Admin API, but they live in the PARTICIPANT migrations — exactly like
-- admin_halls in 001 — because the participant API reads them to render the
-- seat picker, and its migrations run first. Putting them in the admin
-- migrations would leave the participant API querying tables that do not yet
-- exist on a cold start.
-- =============================================================================

-- One seating arrangement per hall. The UNIQUE constraint on hall_id is what
-- enforces "a hall can have one seating arrangement".
CREATE TABLE IF NOT EXISTS admin_hall_seat_maps (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hall_id       UUID NOT NULL UNIQUE REFERENCES admin_halls(id) ON DELETE CASCADE,
    name          TEXT NOT NULL DEFAULT 'Main seating',
    -- Design surface in abstract units; the editor and the client renderer both
    -- treat these as the SVG viewBox so a map looks identical in both.
    canvas_width  INT NOT NULL DEFAULT 1200,
    canvas_height INT NOT NULL DEFAULT 800,
    -- draft   — Ops still designing; not safe to sell against.
    -- published — frozen and usable as an event's seating reference.
    status        TEXT NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft', 'published')),
    -- Bumped on every save so a client can cache-bust a rendered map.
    version       INT NOT NULL DEFAULT 1,
    -- Denormalized count of active seats, recomputed on save. Kept so the halls
    -- list can show "1,043 seats" without aggregating the seats table.
    total_seats   INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Price tiers. `code` is the stable handle the editor and the save payload use
-- to attach seats to a tier, so a tier can be renamed or recoloured without
-- touching a single seat row.
CREATE TABLE IF NOT EXISTS admin_hall_seat_zones (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seat_map_id   UUID NOT NULL REFERENCES admin_hall_seat_maps(id) ON DELETE CASCADE,
    code          TEXT NOT NULL,
    name          TEXT NOT NULL,
    -- Hex colour used by BOTH the Ops editor and the client seat picker, so the
    -- tier a buyer sees is the tier Ops drew.
    color         TEXT NOT NULL DEFAULT '#6a1b9a',
    price         NUMERIC(12,2) NOT NULL DEFAULT 0,
    display_order INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (seat_map_id, code)
);
CREATE INDEX IF NOT EXISTS idx_hall_seat_zones_map ON admin_hall_seat_zones(seat_map_id);

-- One row per physical seat.
CREATE TABLE IF NOT EXISTS admin_hall_seats (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seat_map_id UUID NOT NULL REFERENCES admin_hall_seat_maps(id) ON DELETE CASCADE,
    -- Dropping a zone must not delete the seats in it — they simply fall back
    -- to having no tier, and the editor flags them as unpriced.
    zone_id     UUID REFERENCES admin_hall_seat_zones(id) ON DELETE SET NULL,

    -- Addressing. section + row_label + seat_number is how a human reads a
    -- ticket ("Orchestra, Row C, Seat 14") and is the natural key for upserts.
    section     TEXT NOT NULL DEFAULT 'Main',
    row_label   TEXT NOT NULL,
    -- ROW DISTANCE FROM THE STAGE: 0 == the row closest to the stage, counting
    -- up as you move back. Stored explicitly rather than derived from
    -- row_label, because letters skip (no row "I") and balconies restart their
    -- lettering while still being physically further from the stage.
    row_index   INT NOT NULL DEFAULT 0,
    seat_number INT NOT NULL,
    -- Denormalized display label ("C14"). Generated by the editor; kept so a
    -- ticket or the client can print a seat without re-deriving the format.
    seat_label  TEXT NOT NULL DEFAULT '',

    -- NULL means "inherit the zone price". A non-NULL value is a deliberate
    -- per-seat override set by Ops (e.g. a restricted-view seat sold cheaper).
    price       NUMERIC(12,2),

    -- seat        — a normal sellable seat
    -- wheelchair  — accessible position, sellable, rendered differently
    -- companion   — seat beside a wheelchair position
    -- house       — held back for the house / press, not sellable
    kind        TEXT NOT NULL DEFAULT 'seat'
                CHECK (kind IN ('seat', 'wheelchair', 'companion', 'house')),

    -- Canvas position, same coordinate space as canvas_width/height above.
    pos_x       INT NOT NULL DEFAULT 0,
    pos_y       INT NOT NULL DEFAULT 0,

    -- FALSE = the seat exists in the layout but is blocked (broken, removed for
    -- equipment). It stays in the map so the geometry is preserved, and is
    -- excluded from total_seats and from anything sellable.
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (seat_map_id, section, row_label, seat_number)
);
CREATE INDEX IF NOT EXISTS idx_hall_seats_map  ON admin_hall_seats(seat_map_id);
CREATE INDEX IF NOT EXISTS idx_hall_seats_zone ON admin_hall_seats(zone_id);
-- Serves the client's "show me this section front-to-back" read.
CREATE INDEX IF NOT EXISTS idx_hall_seats_layout
    ON admin_hall_seats(seat_map_id, section, row_index, seat_number);

-- Everything on the canvas that is not a seat. Purely presentational, so these
-- rows carry no pricing and nothing references them — the save endpoint is free
-- to replace them wholesale.
CREATE TABLE IF NOT EXISTS admin_hall_seat_elements (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seat_map_id UUID NOT NULL REFERENCES admin_hall_seat_maps(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL
                CHECK (kind IN ('stage', 'backstage', 'aisle', 'entrance', 'exit', 'label', 'screen')),
    label       TEXT NOT NULL DEFAULT '',
    pos_x       INT NOT NULL DEFAULT 0,
    pos_y       INT NOT NULL DEFAULT 0,
    width       INT NOT NULL DEFAULT 200,
    height      INT NOT NULL DEFAULT 60,
    rotation    INT NOT NULL DEFAULT 0,
    color       TEXT NOT NULL DEFAULT '#0e1237',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_hall_seat_elements_map ON admin_hall_seat_elements(seat_map_id);

-- -----------------------------------------------------------------------------
-- Client-facing read model.
--
-- The participant app (and any future booking flow) should read THIS rather
-- than the base tables, so the COALESCE price rule lives in exactly one place.
-- Only active seats of published maps are exposed — a draft map or a blocked
-- seat can never leak into a seat picker.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE VIEW v_hall_seat_pricing AS
SELECT
    s.id              AS seat_id,
    m.hall_id,
    m.id              AS seat_map_id,
    m.version         AS map_version,
    s.section,
    s.row_label,
    s.row_index,
    s.seat_number,
    s.seat_label,
    s.kind,
    s.pos_x,
    s.pos_y,
    z.code            AS zone_code,
    z.name            AS zone_name,
    z.color           AS zone_color,
    -- The one and only definition of what a seat costs.
    COALESCE(s.price, z.price, 0)::NUMERIC(12,2) AS price,
    -- Lets the client show "special price" without comparing against the tier.
    (s.price IS NOT NULL) AS is_price_override
FROM admin_hall_seats s
JOIN admin_hall_seat_maps m ON m.id = s.seat_map_id
LEFT JOIN admin_hall_seat_zones z ON z.id = s.zone_id
WHERE s.is_active
  AND s.kind <> 'house'
  AND m.status = 'published';
