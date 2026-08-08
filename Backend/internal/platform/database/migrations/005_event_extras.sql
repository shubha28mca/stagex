-- =============================================================================
-- Migration 005 — richer event definition
--
-- The Event Admin create wizard captures more than a round count: named rounds,
-- a judging rubric and a panel of judges. These are stored as JSONB on the
-- event so the participant detail page can render them. Owned by the participant
-- backend (the events schema owner) so both services can rely on the columns.
-- =============================================================================

ALTER TABLE events ADD COLUMN IF NOT EXISTS rounds_detail JSONB; -- [{name, description}]
ALTER TABLE events ADD COLUMN IF NOT EXISTS rubric        JSONB; -- [{criterion, weight}]
ALTER TABLE events ADD COLUMN IF NOT EXISTS judge_ids     JSONB; -- ["<admin_judge uuid>", ...]
