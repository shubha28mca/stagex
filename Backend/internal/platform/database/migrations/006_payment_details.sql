-- Migration 006: Add payment transaction audit columns (Razorpay payment ID and signature)
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS payment_id TEXT,
    ADD COLUMN IF NOT EXISTS signature TEXT;

CREATE INDEX IF NOT EXISTS idx_payments_payment_id ON payments(payment_id);
