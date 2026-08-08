-- =============================================================================
-- Migration 003 — Seed data
--
-- Seeds the admin_ master tables (event types, age bands, taxonomy, coupons,
-- halls, judges, sponsors) plus a few demo events so the participant app has
-- something to show on first run. Idempotent via ON CONFLICT DO NOTHING.
-- =============================================================================

-- ---- Admin: event types --------------------------------------------------
INSERT INTO admin_event_types (code, name, certificate_seal, description) VALUES
 ('national', 'National Championship', 'gold_hologram', 'Flagship national-level championship'),
 ('regional', 'Regional',              'silver',        'Regional qualifier events'),
 ('school',   'School',                'standard',      'School-level talent events'),
 ('online',   'Online-only',           'digital_badge', 'Online submission events')
ON CONFLICT (code) DO NOTHING;

-- ---- Admin: age bands ----------------------------------------------------
INSERT INTO admin_age_bands (code, label, min_age, max_age) VALUES
 ('5-8',   '5 to 8 years',   5,  8),
 ('9-12',  '9 to 12 years',  9,  12),
 ('13-16', '13 to 16 years', 13, 16),
 ('17-21', '17 to 21 years', 17, 21),
 ('22+',   '22 years and above', 22, 200)
ON CONFLICT (code) DO NOTHING;

-- ---- Admin: taxonomy (top-level categories) ------------------------------
INSERT INTO admin_categories (code, name) VALUES
 ('dance',   'Dance'),
 ('music',   'Music'),
 ('vocal',   'Vocal'),
 ('theatre', 'Theatre'),
 ('art',     'Visual Art'),
 ('literary','Literary')
ON CONFLICT (code) DO NOTHING;

-- Sub-categories under Dance and Music.
INSERT INTO admin_categories (code, name, parent_id)
SELECT 'classical-dance', 'Classical Dance', id FROM admin_categories WHERE code='dance'
ON CONFLICT (code) DO NOTHING;
INSERT INTO admin_categories (code, name, parent_id)
SELECT 'folk-dance', 'Folk Dance', id FROM admin_categories WHERE code='dance'
ON CONFLICT (code) DO NOTHING;
INSERT INTO admin_categories (code, name, parent_id)
SELECT 'tabla', 'Tabla', id FROM admin_categories WHERE code='music'
ON CONFLICT (code) DO NOTHING;

-- ---- Admin: coupons ------------------------------------------------------
INSERT INTO admin_coupons (code, discount_type, value, scope, max_uses, valid_until) VALUES
 ('EARLYBIRD20', 'percent', 20, 'global', 1000, now() + interval '90 days'),
 ('FLAT100',     'flat',    100,'global', 500,  now() + interval '60 days')
ON CONFLICT (code) DO NOTHING;

-- ---- Admin: halls --------------------------------------------------------
INSERT INTO admin_halls (name, city, capacity, base_rate, lead_name, lead_contact) VALUES
 ('Nehru Auditorium', 'Mumbai', 1200, 45000, 'R. Naik',   'rnaik@venues.example'),
 ('City Arts Center',  'Delhi',  800,  32000, 'S. Kapoor', 'skapoor@venues.example')
ON CONFLICT DO NOTHING;

-- ---- Admin: judges -------------------------------------------------------
INSERT INTO admin_judges (name, expertise, experience_years, affiliation, is_verified) VALUES
 ('Dr. Meera Rao', 'Classical Dance', 22, 'Kalakshetra Academy', TRUE),
 ('Ustad A. Khan', 'Tabla',           30, 'Delhi Gharana',       TRUE)
ON CONFLICT DO NOTHING;

-- ---- Admin: sponsors -----------------------------------------------------
INSERT INTO admin_sponsors (organisation, tier, contact, committed_amount, scholarship_slots) VALUES
 ('Bright Future Foundation', 'gold',   'contact@brightfuture.example', 500000, 20),
 ('Rhythm Instruments Co.',   'silver', 'sales@rhythm.example',         200000, 5)
ON CONFLICT DO NOTHING;

-- ---- Demo events ---------------------------------------------------------
INSERT INTO events (event_type_id, hall_id, name, tagline, city, mode, rounds, fee, slots_total, slots_filled, start_date, end_date, status, cover_gradient)
SELECT et.id, h.id, 'Nritya Mahotsav', 'National dance championship', 'Mumbai', 'onstage', 3, 499, 200, 128, CURRENT_DATE + 20, CURRENT_DATE + 22, 'open', 'purple'
FROM admin_event_types et, admin_halls h
WHERE et.code='national' AND h.name='Nehru Auditorium'
ON CONFLICT DO NOTHING;

INSERT INTO events (event_type_id, hall_id, name, tagline, city, mode, rounds, fee, slots_total, slots_filled, start_date, end_date, status, cover_gradient)
SELECT et.id, h.id, 'Taal Tarang', 'Tabla & percussion showcase', 'Delhi', 'onstage', 2, 299, 120, 44, CURRENT_DATE + 35, CURRENT_DATE + 36, 'open', 'orange'
FROM admin_event_types et, admin_halls h
WHERE et.code='regional' AND h.name='City Arts Center'
ON CONFLICT DO NOTHING;

-- ---- Event categories for the demo events --------------------------------
INSERT INTO event_categories (event_id, category_id, age_band_id, participation_type, fee)
SELECT e.id, c.id, ab.id, 'solo', 499
FROM events e, admin_categories c, admin_age_bands ab
WHERE e.name='Nritya Mahotsav' AND c.code='classical-dance' AND ab.code='9-12'
ON CONFLICT DO NOTHING;

INSERT INTO event_categories (event_id, category_id, age_band_id, participation_type, fee)
SELECT e.id, c.id, ab.id, 'solo', 499
FROM events e, admin_categories c, admin_age_bands ab
WHERE e.name='Nritya Mahotsav' AND c.code='folk-dance' AND ab.code='13-16'
ON CONFLICT DO NOTHING;

INSERT INTO event_categories (event_id, category_id, age_band_id, participation_type, fee)
SELECT e.id, c.id, ab.id, 'solo', 299
FROM events e, admin_categories c, admin_age_bands ab
WHERE e.name='Taal Tarang' AND c.code='tabla' AND ab.code='9-12'
ON CONFLICT DO NOTHING;
