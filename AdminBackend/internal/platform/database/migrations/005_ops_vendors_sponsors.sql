-- =============================================================================
-- Admin migration 005 — Operations Admin: vendors + event sponsor/vendor income
--
-- A vendor pool (master), and per-event assignments for vendors and sponsors
-- each carrying a cost that is treated as income and added to the event profit
-- (per request). Sponsors reuse the existing admin_sponsors master.
-- =============================================================================

-- Vendor pool owned by Operations (Admin Design §4.3).
CREATE TABLE IF NOT EXISTS admin_vendors (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    service_type TEXT NOT NULL,
    city         TEXT,
    contact      TEXT,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Vendor assigned to an event; cost adds to the event's income.
CREATE TABLE IF NOT EXISTS admin_event_vendors (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    vendor_id    UUID REFERENCES admin_vendors(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    service_type TEXT NOT NULL,
    cost         NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_event_vendors_event ON admin_event_vendors(event_id);

-- Sponsor assigned to an event; cost adds to the event's income.
CREATE TABLE IF NOT EXISTS admin_event_sponsors (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id     UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    sponsor_id   UUID REFERENCES admin_sponsors(id) ON DELETE SET NULL,
    organisation TEXT NOT NULL,
    tier         TEXT NOT NULL DEFAULT '',
    cost         NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_event_sponsors_event ON admin_event_sponsors(event_id);
