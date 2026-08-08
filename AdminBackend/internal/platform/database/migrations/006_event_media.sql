-- =============================================================================
-- Admin migration 006 — event media
--
-- Photos and videos the Event Admin uploads for an event, visible to all its
-- participants. Files are stored on disk under <media_root>/<event_id>/ and
-- this table records each file's public URL. A cloud object store can replace
-- the local folder later without schema changes.
-- =============================================================================

CREATE TABLE IF NOT EXISTS admin_event_media (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,       -- photo | video
    filename   TEXT NOT NULL,       -- stored file name on disk
    url        TEXT NOT NULL,       -- absolute public URL
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_event_media_event ON admin_event_media(event_id);
