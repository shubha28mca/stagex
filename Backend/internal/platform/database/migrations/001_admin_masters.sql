-- =============================================================================
-- Migration 001 — Admin master data (source: StageX_Admin_Design.md)
--
-- These tables hold data owned by the Admin Console (Operation Admin / Super
-- Admin). The participant-facing client tables (migration 002) reference these
-- by foreign key but never write to them. Every table here is prefixed with
-- `admin_` so the two domains are unmistakably separated in the same database.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- provides gen_random_uuid()

-- Event types (National Championship, Regional, School, Online-only). Each type
-- carries default certificate branding — see Admin Design §4.7.
CREATE TABLE admin_event_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            TEXT NOT NULL UNIQUE,          -- machine key, e.g. 'national'
    name            TEXT NOT NULL,                 -- display name
    certificate_seal TEXT NOT NULL,                -- gold_hologram | silver | standard | digital_badge
    description     TEXT NOT NULL DEFAULT '',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Age bands (5-8, 9-12, 13-16, 17-21, 22+). min_age/max_age drive eligibility.
CREATE TABLE admin_age_bands (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT NOT NULL UNIQUE,   -- e.g. '9-12'
    label       TEXT NOT NULL,          -- e.g. '9 to 12 years'
    min_age     INT NOT NULL,
    max_age     INT NOT NULL,           -- use 200 for open-ended (22+)
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT age_band_range CHECK (max_age >= min_age)
);

-- Taxonomy: categories and sub-categories (parent_id self-reference). Versioned
-- so published events keep the version they launched with — Admin Design §4.7.
CREATE TABLE admin_categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   UUID REFERENCES admin_categories(id) ON DELETE CASCADE,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    version     INT NOT NULL DEFAULT 1,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Coupons created by Operations (Ops pool). Participants validate against these
-- at checkout — Admin Design §4.9, Client rule 5.
CREATE TABLE admin_coupons (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          TEXT NOT NULL UNIQUE,
    discount_type TEXT NOT NULL,          -- percent | flat | sponsored_100
    value         NUMERIC(10,2) NOT NULL DEFAULT 0,   -- percent (0-100) or flat rupees
    scope         TEXT NOT NULL DEFAULT 'global',     -- global | event | category
    scope_ref_id  UUID,                   -- event/category id when scope != global
    max_uses      INT NOT NULL DEFAULT 0, -- 0 = unlimited
    used_count    INT NOT NULL DEFAULT 0,
    valid_from    TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until   TIMESTAMPTZ,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hall / venue registry (master data) — Admin Design §4.2.
CREATE TABLE admin_halls (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    city          TEXT NOT NULL,
    capacity      INT NOT NULL DEFAULT 0,
    base_rate     NUMERIC(12,2) NOT NULL DEFAULT 0,   -- rupees per day
    lead_name     TEXT,
    lead_contact  TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Judge pool (Ops-owned) — Admin Design §4.6.
CREATE TABLE admin_judges (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    expertise    TEXT NOT NULL,
    experience_years INT NOT NULL DEFAULT 0,
    affiliation  TEXT,               -- drives conflict-of-interest checks
    is_verified  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sponsor profiles (Ops-owned) — Admin Design §4.9.
CREATE TABLE admin_sponsors (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation   TEXT NOT NULL,
    tier           TEXT NOT NULL,     -- platinum | gold | silver | impact
    contact        TEXT,
    committed_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    scholarship_slots INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_categories_parent ON admin_categories(parent_id);
CREATE INDEX idx_admin_coupons_code ON admin_coupons(code);
CREATE INDEX idx_admin_halls_city ON admin_halls(city);
