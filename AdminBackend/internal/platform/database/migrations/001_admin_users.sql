-- =============================================================================
-- Admin migration 001 — admin_users
--
-- Accounts for the Admin Console. Scope is limited to two roles (Super Admin is
-- out of scope): 'ops' (Operational Admin) and 'event' (Event Admin). Mock
-- accounts are seeded by the service on boot (bcrypt hashes computed in Go).
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS admin_users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('ops','event')),
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
