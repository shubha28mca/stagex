-- =============================================================================
-- Migration 008 — Ticket booking (participant side)
--
-- Lets a logged-in family pick seats from a hall's published arrangement
-- (migration 006), hold them, pay, and later cancel.
--
--   event_seat_status  the per-event state of one seat — THE concurrency point
--   bookings           one purchase attempt, and its whole lifecycle
--   booking_items      one row per seat bought, as an immutable snapshot
--   booking_payments   one row per payment attempt
--
-- ---------------------------------------------------------------------------
-- CONCURRENCY MODEL
--
-- Two people must never buy the same seat. Rather than relying on application
-- checks, every claim funnels through ONE lockable row per (event, seat) in
-- event_seat_status, with UNIQUE (event_id, seat_id). To hold seats a
-- transaction does:
--
--   1. INSERT ... ON CONFLICT DO NOTHING   (materialize the rows it needs)
--   2. SELECT ... ORDER BY seat_id FOR UPDATE
--      ^ the ORDER BY matters: locking in a deterministic order means two
--        overlapping requests queue instead of deadlocking.
--   3. re-check each seat, then claim it
--
-- Whoever commits first wins; the loser sees `held` and is told which seats
-- went. There is no window in which both succeed.
--
-- EXPIRY is judged by TIME, never by a flag alone, so a crashed sweeper can
-- never wedge a seat. A row counts as taken only while
-- `status='held' AND hold_expires_at > now()`. Reads apply that same predicate,
-- so availability is correct the instant a hold lapses — the background sweeper
-- only tidies rows up, it is not what makes the rule true.
--
-- THE PAYMENT-IN-FLIGHT EDGE CASE: a hold must not lapse while the gateway is
-- mid-authorisation, or a buyer could be charged for a seat someone else just
-- took. A booking in `payment_processing` therefore keeps its seats through a
-- grace window past hold_expires_at (PAYMENT_GRACE in the API config). If a
-- payment still lands after even that, confirmation re-checks the seats and, if
-- any were lost, cancels the booking with an offline refund rather than
-- double-selling.
-- ---------------------------------------------------------------------------
-- REFUNDS ARE OFFLINE ONLY. Nothing here moves money back; cancelling records
-- what is owed (`refund_status='pending_offline'`) for finance to settle by
-- hand, and admin marks it completed.
-- =============================================================================

-- ---- bookings ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS bookings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Short human handle shown to the buyer and quoted to support ("SX-7QK2M4").
    reference     TEXT NOT NULL UNIQUE,
    -- RESTRICT, not CASCADE: an event with money taken against it must not be
    -- deletable out from under its bookings.
    event_id      UUID NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    family_id     UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    -- Ties the booking back to the seats held for it in event_seat_status.
    hold_group_id UUID NOT NULL,

    -- pending_payment    seats held, buyer has not started paying
    -- payment_processing gateway is mid-authorisation (grace window applies)
    -- confirmed          paid; seats are booked
    -- cancelled          cancelled by the family or an admin; refund offline
    -- expired            the hold lapsed before payment completed
    status        TEXT NOT NULL DEFAULT 'pending_payment'
                  CHECK (status IN ('pending_payment', 'payment_processing',
                                    'confirmed', 'cancelled', 'expired')),

    seat_count    INT NOT NULL DEFAULT 0,
    subtotal      NUMERIC(12,2) NOT NULL DEFAULT 0,
    discount      NUMERIC(12,2) NOT NULL DEFAULT 0,
    total         NUMERIC(12,2) NOT NULL DEFAULT 0,
    coupon_code   TEXT NOT NULL DEFAULT '',

    -- The deadline the buyer is racing. Set when the seats were held, and the
    -- single source of truth for whether this booking still owns them.
    hold_expires_at TIMESTAMPTZ NOT NULL,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at  TIMESTAMPTZ,
    cancelled_at  TIMESTAMPTZ,

    -- Cancellation is a permanent part of the record: the booking is never
    -- deleted, so the family keeps a full history of what they bought and gave up.
    cancellation_reason TEXT NOT NULL DEFAULT '',
    cancellation_note   TEXT NOT NULL DEFAULT '',
    cancelled_by        TEXT NOT NULL DEFAULT ''
                        CHECK (cancelled_by IN ('', 'family', 'admin', 'system')),

    -- Money is only ever returned by hand, so this is a work item for finance
    -- rather than a gateway instruction.
    refund_status TEXT NOT NULL DEFAULT 'not_applicable'
                  CHECK (refund_status IN ('not_applicable', 'pending_offline', 'completed')),
    refund_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    refund_note   TEXT NOT NULL DEFAULT '',
    refunded_at   TIMESTAMPTZ,

    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bookings_family ON bookings(family_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_bookings_event  ON bookings(event_id, status);
CREATE INDEX IF NOT EXISTS idx_bookings_hold   ON bookings(hold_group_id);
-- Drives the sweeper: find only the bookings that can still time out.
CREATE INDEX IF NOT EXISTS idx_bookings_expiry ON bookings(hold_expires_at)
    WHERE status IN ('pending_payment', 'payment_processing');

-- ---- booking_items ----------------------------------------------------------
-- One row per seat. Every human-readable field is a SNAPSHOT taken at purchase
-- time: Ops may redesign the hall's arrangement later, and a past ticket must
-- still print the row and price that were actually sold. seat_id is kept as a
-- soft link for reporting and goes NULL if that seat is ever removed.
CREATE TABLE IF NOT EXISTS booking_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id  UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    seat_id     UUID REFERENCES admin_hall_seats(id) ON DELETE SET NULL,

    section     TEXT NOT NULL,
    row_label   TEXT NOT NULL,
    -- Distance from the stage, snapshotted — this is what "how good was the
    -- seat" means on a historical ticket.
    row_index   INT NOT NULL DEFAULT 0,
    seat_number INT NOT NULL,
    seat_label  TEXT NOT NULL,
    zone_code   TEXT NOT NULL DEFAULT '',
    zone_name   TEXT NOT NULL DEFAULT '',
    -- The price actually charged for this seat.
    price       NUMERIC(12,2) NOT NULL DEFAULT 0,

    status      TEXT NOT NULL DEFAULT 'confirmed'
                CHECK (status IN ('confirmed', 'cancelled')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_booking_items_booking ON booking_items(booking_id);
CREATE INDEX IF NOT EXISTS idx_booking_items_seat    ON booking_items(seat_id);

-- ---- event_seat_status ------------------------------------------------------
-- The concurrency control point. A row appears the first time a seat is held or
-- booked for an event; NO ROW MEANS AVAILABLE, so an event with no sales costs
-- nothing to store.
CREATE TABLE IF NOT EXISTS event_seat_status (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    seat_id         UUID NOT NULL REFERENCES admin_hall_seats(id) ON DELETE CASCADE,

    status          TEXT NOT NULL DEFAULT 'available'
                    CHECK (status IN ('available', 'held', 'booked')),

    -- Who holds it and until when. `status='held'` alone means nothing: a hold
    -- counts only while hold_expires_at is still in the future.
    held_by         UUID REFERENCES families(id) ON DELETE SET NULL,
    hold_group_id   UUID,
    hold_expires_at TIMESTAMPTZ,

    booking_id      UUID REFERENCES bookings(id) ON DELETE SET NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- The constraint the whole design rests on: one state row per seat per
    -- event, so concurrent claims contend on a single lockable row.
    UNIQUE (event_id, seat_id)
);
CREATE INDEX IF NOT EXISTS idx_ess_event ON event_seat_status(event_id, status);
-- Serves the sweeper's "which holds have lapsed" scan.
CREATE INDEX IF NOT EXISTS idx_ess_hold_expiry ON event_seat_status(hold_expires_at)
    WHERE status = 'held';
CREATE INDEX IF NOT EXISTS idx_ess_group ON event_seat_status(hold_group_id);

-- ---- booking_payments -------------------------------------------------------
-- One row per attempt, so a retry never overwrites the evidence of the last try.
CREATE TABLE IF NOT EXISTS booking_payments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id     UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    provider       TEXT NOT NULL DEFAULT 'mock',
    -- The gateway's handle. UNIQUE makes confirmation idempotent: a webhook
    -- delivered twice cannot be counted twice.
    provider_ref   TEXT NOT NULL UNIQUE,
    amount         NUMERIC(12,2) NOT NULL DEFAULT 0,
    attempt        INT NOT NULL DEFAULT 1,
    status         TEXT NOT NULL DEFAULT 'processing'
                   CHECK (status IN ('processing', 'succeeded', 'failed')),
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_booking_payments_booking ON booking_payments(booking_id, created_at DESC);

-- -----------------------------------------------------------------------------
-- Admin read model: "how many tickets are booked for this event?"
--
-- Cancelled seats are counted separately rather than subtracted, because Ops
-- needs both numbers — what is sold now, and what came back. refund_pending is
-- the finance work queue, since refunds are settled by hand.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE VIEW v_event_booking_stats AS
SELECT
    e.id                                       AS event_id,
    e.name                                     AS event_name,
    e.start_date,
    e.status                                   AS event_status,
    count(DISTINCT b.id) FILTER (WHERE b.status = 'confirmed')            AS confirmed_bookings,
    count(DISTINCT b.id) FILTER (WHERE b.status = 'cancelled')            AS cancelled_bookings,
    count(DISTINCT b.id) FILTER (WHERE b.status IN ('pending_payment',
                                                    'payment_processing')) AS in_progress_bookings,
    count(bi.id) FILTER (WHERE b.status = 'confirmed'
                           AND bi.status = 'confirmed')                    AS tickets_booked,
    count(bi.id) FILTER (WHERE bi.status = 'cancelled')                    AS tickets_cancelled,
    COALESCE(sum(bi.price) FILTER (WHERE b.status = 'confirmed'
                                     AND bi.status = 'confirmed'), 0)::NUMERIC(14,2) AS gross_revenue,
    COALESCE(sum(b.refund_amount) FILTER (WHERE b.refund_status = 'pending_offline'), 0)::NUMERIC(14,2)
                                                                           AS refund_pending,
    COALESCE(sum(b.refund_amount) FILTER (WHERE b.refund_status = 'completed'), 0)::NUMERIC(14,2)
                                                                           AS refund_completed
FROM events e
LEFT JOIN bookings b      ON b.event_id = e.id
LEFT JOIN booking_items bi ON bi.booking_id = b.id
GROUP BY e.id, e.name, e.start_date, e.status;
