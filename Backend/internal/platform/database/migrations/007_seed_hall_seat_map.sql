-- =============================================================================
-- Migration 007 — sample seating arrangement (demo / visualization data)
--
-- Builds one complete, published seat map for the seeded "Nehru Auditorium" so
-- the Ops seat-map designer has something real to open on a fresh database:
--
--   482 seats over 3 sections and 18 rows, 4 price tiers, a stage + backstage,
--   plus deliberate edge cases — per-seat price overrides on restricted-view
--   side seats, wheelchair/companion positions, blocked seats and house seats.
--
-- "City Arts Center" is intentionally left WITHOUT a map, so the
-- create-an-arrangement flow is also demonstrable on a fresh database.
--
-- Every statement is driven off a SELECT against admin_halls, so on a database
-- where the participant seed never ran this migration is a harmless no-op
-- rather than a failure.
-- =============================================================================

-- ---- The map itself ---------------------------------------------------------
INSERT INTO admin_hall_seat_maps (hall_id, name, canvas_width, canvas_height, status, total_seats)
SELECT h.id, 'Main auditorium seating', 1200, 800, 'published', 0
FROM admin_halls h
WHERE h.name = 'Nehru Auditorium'
ON CONFLICT (hall_id) DO NOTHING;

-- ---- Price tiers ------------------------------------------------------------
INSERT INTO admin_hall_seat_zones (seat_map_id, code, name, color, price, display_order)
SELECT m.id, z.code, z.name, z.color, z.price, z.display_order
FROM admin_hall_seat_maps m
JOIN admin_halls h ON h.id = m.hall_id AND h.name = 'Nehru Auditorium'
CROSS JOIN (VALUES
    ('vip',      'VIP Front',      '#6a1b9a', 2500.00, 1),
    ('premium',  'Premium Stalls', '#ff7a3c', 1500.00, 2),
    ('standard', 'Mezzanine',      '#37b6ff',  900.00, 3),
    ('balcony',  'Balcony',        '#2ecc8a',  450.00, 4)
) AS z(code, name, color, price, display_order)
ON CONFLICT (seat_map_id, code) DO NOTHING;

-- ---- Seats ------------------------------------------------------------------
-- Row geometry is described once in `layout`, then cross-joined with a seat
-- number series. row_index is the DISTANCE FROM THE STAGE (0 = front row) and
-- runs unbroken 0..17 even though the letters skip I and O, which is exactly
-- why row_index is stored rather than derived from row_label.
WITH map AS (
    SELECT m.id
    FROM admin_hall_seat_maps m
    JOIN admin_halls h ON h.id = m.hall_id
    WHERE h.name = 'Nehru Auditorium'
),
layout AS (
    SELECT * FROM (VALUES
        -- section,      row, row_index, seats, zone,       start_x, pos_y
        ('Orchestra',    'A',  0, 24, 'vip',       301, 170),
        ('Orchestra',    'B',  1, 24, 'vip',       301, 200),
        ('Orchestra',    'C',  2, 24, 'vip',       301, 230),
        ('Orchestra',    'D',  3, 24, 'premium',   301, 260),
        ('Orchestra',    'E',  4, 24, 'premium',   301, 290),
        ('Orchestra',    'F',  5, 24, 'premium',   301, 320),
        ('Orchestra',    'G',  6, 24, 'premium',   301, 350),
        ('Orchestra',    'H',  7, 24, 'premium',   301, 380),
        ('Mezzanine',    'J',  8, 28, 'standard',  249, 440),
        ('Mezzanine',    'K',  9, 28, 'standard',  249, 470),
        ('Mezzanine',    'L', 10, 28, 'standard',  249, 500),
        ('Mezzanine',    'M', 11, 28, 'standard',  249, 530),
        ('Mezzanine',    'N', 12, 28, 'standard',  249, 560),
        ('Balcony',      'P', 13, 30, 'balcony',   223, 620),
        ('Balcony',      'Q', 14, 30, 'balcony',   223, 650),
        ('Balcony',      'R', 15, 30, 'balcony',   223, 680),
        ('Balcony',      'S', 16, 30, 'balcony',   223, 710),
        ('Balcony',      'T', 17, 30, 'balcony',   223, 740)
    ) AS t(section, row_label, row_index, seats, zone_code, start_x, pos_y)
)
INSERT INTO admin_hall_seats
    (seat_map_id, zone_id, section, row_label, row_index, seat_number, seat_label,
     price, kind, pos_x, pos_y, is_active)
SELECT
    map.id,
    z.id,
    l.section,
    l.row_label,
    l.row_index,
    n.seat_number,
    l.row_label || n.seat_number::text,
    -- Per-seat override: the two seats at each end of the three VIP rows have a
    -- restricted view of the stage, so they sell below their tier.
    CASE
      WHEN l.zone_code = 'vip' AND (n.seat_number <= 2 OR n.seat_number > l.seats - 2)
      THEN 1800.00
    END,
    -- A pair of wheelchair positions plus companion seats at the back of the
    -- rear orchestra row, and the last two balcony seats held for the house.
    CASE
      WHEN l.row_label = 'H' AND n.seat_number IN (1, 2)              THEN 'wheelchair'
      WHEN l.row_label = 'H' AND n.seat_number IN (3, 4)              THEN 'companion'
      WHEN l.row_label = 'T' AND n.seat_number > l.seats - 2          THEN 'house'
      ELSE 'seat'
    END,
    l.start_x + (n.seat_number - 1) * 26,
    l.pos_y,
    -- Two blocked seats mid-row F: a lighting desk stands there. They stay in
    -- the map so the geometry is right, but are not sellable.
    NOT (l.row_label = 'F' AND n.seat_number IN (12, 13))
FROM map
CROSS JOIN layout l
CROSS JOIN LATERAL generate_series(1, l.seats) AS n(seat_number)
LEFT JOIN admin_hall_seat_zones z ON z.seat_map_id = map.id AND z.code = l.zone_code
ON CONFLICT (seat_map_id, section, row_label, seat_number) DO NOTHING;

-- ---- Stage, backstage and circulation --------------------------------------
INSERT INTO admin_hall_seat_elements (seat_map_id, kind, label, pos_x, pos_y, width, height, color)
SELECT m.id, e.kind, e.label, e.pos_x, e.pos_y, e.width, e.height, e.color
FROM admin_hall_seat_maps m
JOIN admin_halls h ON h.id = m.hall_id AND h.name = 'Nehru Auditorium'
CROSS JOIN (VALUES
    ('backstage', 'BACKSTAGE',      450,  20, 300, 40, '#171c4f'),
    ('stage',     'STAGE',          400,  70, 400, 80, '#0e1237'),
    ('aisle',     '',               588, 170, 24, 250, '#e9e6f2'),
    ('label',     'MEZZANINE',      249, 410, 140, 22, '#6b6785'),
    ('label',     'BALCONY',        223, 592, 120, 22, '#6b6785'),
    ('entrance',  'ENTRANCE LEFT',   60, 745, 130, 34, '#2ecc8a'),
    ('entrance',  'ENTRANCE RIGHT', 1010, 745, 130, 34, '#2ecc8a')
) AS e(kind, label, pos_x, pos_y, width, height, color);

-- ---- Refresh the denormalized seat count -----------------------------------
-- Mirrors exactly what the save endpoint does, so the seeded value and an
-- app-written value can never disagree.
UPDATE admin_hall_seat_maps m
SET total_seats = (
        SELECT count(*) FROM admin_hall_seats s
        WHERE s.seat_map_id = m.id AND s.is_active AND s.kind <> 'house'
    ),
    updated_at = now()
WHERE EXISTS (SELECT 1 FROM admin_hall_seats s WHERE s.seat_map_id = m.id);
