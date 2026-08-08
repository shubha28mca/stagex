-- =============================================================================
-- Admin migration 003 — Event Admin operational features
--
-- Crew assigned to an event, ad-hoc notifications (broadcasts) participants can
-- read, and per-event notification-trigger configuration. All admin-owned so
-- they carry the admin_ prefix. Guarded on the participant-owned events table.
-- =============================================================================

CREATE TABLE IF NOT EXISTS admin_event_crew (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    role       TEXT NOT NULL,      -- Stage / Registration / Green Room / AV / Security / Hospitality
    contact    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_event_crew_event ON admin_event_crew(event_id);

-- Broadcasts the Event Admin sends; participants of the event read these.
CREATE TABLE IF NOT EXISTS admin_notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    audience   TEXT NOT NULL DEFAULT 'all',   -- all | paid | pending
    title      TEXT NOT NULL,
    message    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_notifications_event ON admin_notifications(event_id);

-- Per-event automatic-trigger channel toggles (in-app/email/sms/whatsapp).
CREATE TABLE IF NOT EXISTS admin_notification_config (
    event_id   UUID PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    config     JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
