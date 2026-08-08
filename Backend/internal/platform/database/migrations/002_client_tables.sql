-- =============================================================================
-- Migration 002 — Client (participant-facing) tables
--
-- Source: ClientDesignWeb.md and ClientDesignMobile.md. These tables reference
-- the admin_ master tables (migration 001) by foreign key. The account model is
-- "one phone = one family, many people" (ClientDesignWeb §2).
-- =============================================================================

-- A family account, keyed to one verified mobile number.
CREATE TABLE families (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone          TEXT NOT NULL UNIQUE,      -- 10-digit Indian mobile
    display_name   TEXT NOT NULL DEFAULT '',
    password_hash  TEXT NOT NULL DEFAULT '',  -- bcrypt; empty until password set
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One-time passwords for registration/login. A row is consumed on verify.
CREATE TABLE otp_challenges (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone       TEXT NOT NULL,
    code_hash   TEXT NOT NULL,          -- never store the raw OTP
    purpose     TEXT NOT NULL,          -- register | login | reset
    attempts    INT NOT NULL DEFAULT 0, -- max 3 then locked (design §3.1)
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_otp_phone ON otp_challenges(phone);

-- People under a family. mandatory: name, dob, gender, aadhaar; optional:
-- school, city, guru. Aadhaar is stored encrypted and only ever shown masked.
CREATE TABLE people (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id      UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    dob            DATE NOT NULL,
    gender         TEXT NOT NULL,          -- male | female | other
    aadhaar_enc    BYTEA,                  -- AES-GCM ciphertext (never plaintext)
    aadhaar_masked TEXT,                   -- e.g. 'XXXX-XXXX-1234'
    relationship   TEXT NOT NULL DEFAULT 'other',
    school         TEXT,
    city           TEXT,
    guru           TEXT,
    photo_url      TEXT,                   -- GLOBAL profile photo (fallback only)
    bio            TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_people_family ON people(family_id);

-- Events. Reference the admin event-type master; venue links the hall master.
CREATE TABLE events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type_id  UUID REFERENCES admin_event_types(id),
    hall_id        UUID REFERENCES admin_halls(id),
    name           TEXT NOT NULL,
    tagline        TEXT NOT NULL DEFAULT '',
    city           TEXT NOT NULL,
    mode           TEXT NOT NULL DEFAULT 'onstage',   -- onstage | online
    rounds         INT NOT NULL DEFAULT 1,
    fee            NUMERIC(10,2) NOT NULL DEFAULT 0,
    slots_total    INT NOT NULL DEFAULT 0,
    slots_filled   INT NOT NULL DEFAULT 0,
    start_date     DATE NOT NULL,
    end_date       DATE NOT NULL,
    status         TEXT NOT NULL DEFAULT 'open',       -- open | live | completed | draft
    cover_gradient TEXT NOT NULL DEFAULT 'purple',     -- theme hint for the card
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_city ON events(city);
CREATE INDEX idx_events_status ON events(status);

-- The categories offered within an event, each bound to an age band + type.
CREATE TABLE event_categories (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id           UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    category_id        UUID NOT NULL REFERENCES admin_categories(id),
    age_band_id        UUID NOT NULL REFERENCES admin_age_bands(id),
    participation_type TEXT NOT NULL DEFAULT 'solo',    -- solo | duet | group
    min_group_size     INT NOT NULL DEFAULT 1,
    max_group_size     INT NOT NULL DEFAULT 1,
    fee                NUMERIC(10,2) NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_event_categories_event ON event_categories(event_id);

-- A registration is one checkout: one family, one event, one or more entries.
CREATE TABLE registrations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id     UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    event_id      UUID NOT NULL REFERENCES events(id),
    status        TEXT NOT NULL DEFAULT 'pending',  -- pending | paid | held | cancelled
    coupon_code   TEXT,
    subtotal      NUMERIC(12,2) NOT NULL DEFAULT 0,
    discount      NUMERIC(12,2) NOT NULL DEFAULT 0,
    total         NUMERIC(12,2) NOT NULL DEFAULT 0,
    held_until    TIMESTAMPTZ,                      -- 30-min hold on payment fail
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_registrations_family ON registrations(family_id);

-- One entry = one person in one event category. Carries the event-scoped photo
-- and the per-event note (both never shared across events — Client §2, §6).
CREATE TABLE entries (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    registration_id    UUID NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
    person_id          UUID NOT NULL REFERENCES people(id),
    event_category_id  UUID NOT NULL REFERENCES event_categories(id),
    entry_code         TEXT NOT NULL UNIQUE,        -- e.g. 'NM-3241'
    event_photo_url    TEXT,                        -- event-scoped, never global
    note               TEXT,                        -- per participant per event
    status             TEXT NOT NULL DEFAULT 'active',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_entries_registration ON entries(registration_id);
CREATE INDEX idx_entries_person ON entries(person_id);

-- Payment attempts against a registration (Razorpay in production; mock here).
CREATE TABLE payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    registration_id UUID NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL DEFAULT 'razorpay',
    order_ref       TEXT NOT NULL,          -- provider order id
    amount          NUMERIC(12,2) NOT NULL,
    status          TEXT NOT NULL DEFAULT 'created',  -- created | success | failed
    attempt         INT NOT NULL DEFAULT 1,           -- max 3 (design rule 6)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_registration ON payments(registration_id);

-- Certificates earned by people. QR-verifiable and downloadable (Client §8).
CREATE TABLE certificates (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id     UUID NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    event_id      UUID NOT NULL REFERENCES events(id),
    category_name TEXT NOT NULL,
    position      TEXT NOT NULL,          -- gold | silver | bronze | participation
    cert_code     TEXT NOT NULL UNIQUE,   -- printed on the certificate + QR
    file_url      TEXT,                   -- downloadable PDF
    issued_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_certificates_person ON certificates(person_id);

-- Feedback: one submission per family per event, post-completion (Client §9).
CREATE TABLE feedback (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id    UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    event_id     UUID NOT NULL REFERENCES events(id),
    ratings      JSONB NOT NULL,          -- {overall, judges, venue, food, schedule, sponsors}
    comment      TEXT,
    anonymous    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (family_id, event_id)
);

-- Registered push devices (mobile) — ClientDesignMobile §3.4.
CREATE TABLE devices (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id    UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    push_token   TEXT NOT NULL,
    platform     TEXT NOT NULL,          -- android | ios | web
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (push_token)
);
