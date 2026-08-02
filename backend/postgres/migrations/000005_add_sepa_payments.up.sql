-- Extend the existing ledger into a simulation-only EUR/SEPA payment domain.
-- Existing migrations stay immutable; this migration is safe to run once on an
-- already populated demo database.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS full_name TEXT NOT NULL DEFAULT '';

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS iban TEXT,
    ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT 'GIROKONTO',
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'ACTIVE',
    ADD COLUMN IF NOT EXISTS available_balance NUMERIC(19,4),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_account_type_check,
    ADD CONSTRAINT accounts_account_type_check
        CHECK (account_type IN ('GIROKONTO', 'SPARKONTO', 'SETTLEMENT')),
    DROP CONSTRAINT IF EXISTS accounts_status_check,
    ADD CONSTRAINT accounts_status_check
        CHECK (status IN ('ACTIVE', 'BLOCKED', 'CLOSED'));

-- This bank code is reserved by this project purely as a demo identifier. It
-- must never be used to route a real payment.
CREATE OR REPLACE FUNCTION demo_iban_for_sequence(sequence_value BIGINT)
RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    bban TEXT;
    checksum INTEGER;
BEGIN
    bban := '99999999' || LPAD(sequence_value::TEXT, 10, '0');
    checksum := 98 - MOD((bban || '131400')::NUMERIC, 97);
    RETURN 'DE' || LPAD(checksum::TEXT, 2, '0') || bban;
END;
$$;

WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) AS sequence_value
    FROM accounts
    WHERE iban IS NULL
)
UPDATE accounts AS account
SET iban = demo_iban_for_sequence(numbered.sequence_value),
    currency = 'EUR',
    available_balance = account.balance,
    account_type = CASE WHEN account.is_system THEN 'SETTLEMENT' ELSE 'GIROKONTO' END,
    updated_at = CURRENT_TIMESTAMP
FROM numbered
WHERE account.id = numbered.id;

ALTER TABLE accounts
    ALTER COLUMN iban SET NOT NULL,
    ALTER COLUMN currency SET DEFAULT 'EUR',
    ALTER COLUMN available_balance SET DEFAULT 0.0000,
    ALTER COLUMN available_balance SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS accounts_iban_unique_idx ON accounts (iban);
CREATE INDEX IF NOT EXISTS accounts_owner_status_idx ON accounts (owner_id, status);

-- Keep the settlement account in the same simulated currency as all new demo
-- payment accounts. No FX conversion is performed.
UPDATE accounts
SET currency = 'EUR', account_type = 'SETTLEMENT', updated_at = CURRENT_TIMESTAMP
WHERE is_system = TRUE AND name = 'Settlement Account';

CREATE TABLE IF NOT EXISTS beneficiaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    iban TEXT NOT NULL,
    bic TEXT,
    category TEXT,
    is_demo BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (owner_id, iban)
);

CREATE TABLE IF NOT EXISTS standing_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    beneficiary_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    beneficiary_name TEXT NOT NULL,
    beneficiary_iban TEXT NOT NULL,
    beneficiary_bic TEXT,
    amount NUMERIC(19,4) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    purpose TEXT CHECK (char_length(purpose) <= 140),
    creditor_reference TEXT,
    transfer_type TEXT NOT NULL DEFAULT 'STANDARD'
        CHECK (transfer_type IN ('STANDARD', 'INSTANT')),
    frequency TEXT NOT NULL
        CHECK (frequency IN ('WEEKLY', 'MONTHLY', 'QUARTERLY', 'YEARLY')),
    start_date DATE NOT NULL,
    end_date DATE,
    max_occurrences INTEGER CHECK (max_occurrences IS NULL OR max_occurrences > 0),
    occurrences_created INTEGER NOT NULL DEFAULT 0 CHECK (occurrences_created >= 0),
    next_execution_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'PAUSED', 'CANCELLED', 'COMPLETED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS standing_orders_due_idx
    ON standing_orders (next_execution_at)
    WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS payment_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    source_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    beneficiary_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    standing_order_id UUID REFERENCES standing_orders(id) ON DELETE SET NULL,
    beneficiary_name TEXT NOT NULL,
    beneficiary_iban TEXT NOT NULL,
    beneficiary_bic TEXT,
    amount NUMERIC(19,4) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    payment_kind TEXT NOT NULL
        CHECK (payment_kind IN ('UMBUCHUNG', 'INTERNAL', 'SEPA', 'SEPA_INSTANT')),
    schedule_type TEXT NOT NULL DEFAULT 'IMMEDIATE'
        CHECK (schedule_type IN ('IMMEDIATE', 'SCHEDULED', 'STANDING')),
    purpose TEXT CHECK (char_length(purpose) <= 140),
    creditor_reference TEXT,
    end_to_end_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    requested_execution_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    vop_result TEXT NOT NULL CHECK (vop_result IN ('MATCH', 'CLOSE_MATCH', 'NO_MATCH', 'OTHER')),
    vop_suggested_name TEXT,
    vop_overridden BOOLEAN NOT NULL DEFAULT FALSE,
    vop_decision_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'AWAITING_CONFIRMATION'
        CHECK (status IN ('DRAFT', 'AWAITING_CONFIRMATION', 'SCHEDULED', 'PROCESSING', 'BOOKED', 'FAILED', 'CANCELLED')),
    reject_code TEXT,
    failure_reason TEXT,
    ledger_transaction_id UUID,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    processing_started_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at TIMESTAMPTZ,
    UNIQUE (owner_id, idempotency_key),
    UNIQUE (end_to_end_id)
);

CREATE INDEX IF NOT EXISTS payment_orders_owner_created_idx
    ON payment_orders (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS payment_orders_due_idx
    ON payment_orders (requested_execution_at)
    WHERE status = 'SCHEDULED';

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    payment_order_id UUID REFERENCES payment_orders(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    event_data JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS audit_events_owner_id_idx
    ON audit_events (owner_id, id DESC);
CREATE INDEX IF NOT EXISTS audit_events_payment_order_idx
    ON audit_events (payment_order_id, id);

ALTER TABLE entries
    ADD COLUMN IF NOT EXISTS payment_order_id UUID REFERENCES payment_orders(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS counterparty_name TEXT,
    ADD COLUMN IF NOT EXISTS counterparty_iban TEXT,
    ADD COLUMN IF NOT EXISTS purpose TEXT,
    ADD COLUMN IF NOT EXISTS category TEXT,
    ADD COLUMN IF NOT EXISTS booking_date TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS execution_date TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS entries_payment_order_idx ON entries (payment_order_id);

DROP FUNCTION demo_iban_for_sequence(BIGINT);
