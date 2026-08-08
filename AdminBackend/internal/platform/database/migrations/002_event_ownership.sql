-- =============================================================================
-- Admin migration 002 — event ownership
--
-- Event Admins own the events they create; Operations sees all. We add a
-- created_by column to the participant-owned `events` table so ownership can be
-- attributed. Guarded so it is a no-op if the participant API has not yet
-- created the events table (the compose ordering ensures it has).
-- =============================================================================

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'events') THEN
        ALTER TABLE events ADD COLUMN IF NOT EXISTS created_by UUID;
    END IF;
END $$;
