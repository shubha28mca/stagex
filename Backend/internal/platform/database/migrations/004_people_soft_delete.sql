-- =============================================================================
-- Migration 004 — Soft-delete for people
--
-- A person the family removes while still attached to an incomplete event must
-- be retained (shown grayed-out) until that event completes, so entries and
-- certificates keep their reference. deleted_at marks such retained rows.
-- =============================================================================

ALTER TABLE people ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_people_deleted ON people(deleted_at);
