-- =============================================================================
-- Admin migration 004 — Operations Admin: crew pool, crew cost, event expenses
--
-- A reusable crew pool (with day cost) that Ops assigns to events, a cost column
-- on the per-event crew assignment, and free-form additional event expenses.
-- These feed the per-event P&L (Admin Design §4.1, §4.10).
-- =============================================================================

-- Reusable crew pool owned by Operations.
CREATE TABLE IF NOT EXISTS admin_crew (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    role       TEXT NOT NULL,
    cost       NUMERIC(12,2) NOT NULL DEFAULT 0,   -- engagement cost per event
    contact    TEXT,
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The per-event crew assignment (admin_event_crew, from migration 003) gains a
-- cost snapshot and an optional link back to the pool member it came from.
ALTER TABLE admin_event_crew ADD COLUMN IF NOT EXISTS cost    NUMERIC(12,2) NOT NULL DEFAULT 0;
ALTER TABLE admin_event_crew ADD COLUMN IF NOT EXISTS crew_id UUID REFERENCES admin_crew(id) ON DELETE SET NULL;

-- Additional, free-form expenses booked against an event (with a comment).
CREATE TABLE IF NOT EXISTS admin_event_expenses (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    amount     NUMERIC(12,2) NOT NULL DEFAULT 0,
    comment    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_event_expenses_event ON admin_event_expenses(event_id);
