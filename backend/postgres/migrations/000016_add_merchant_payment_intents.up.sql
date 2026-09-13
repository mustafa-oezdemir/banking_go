-- Merchant payment intents are Banking-owned, immutable checkout instructions.
-- The browser receives only an opaque UUID; amount, merchant, recipient IBAN,
-- and reference remain server-side until the customer explicitly approves.
CREATE TABLE merchants (
    id TEXT PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{2,63}$'),
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 140),
    beneficiary_account_id UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE RESTRICT,
    return_url TEXT NOT NULL CHECK (return_url ~ '^https?://'),
    webhook_url TEXT,
    webhook_secret TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((webhook_url IS NULL AND webhook_secret IS NULL) OR
           (webhook_url ~ '^https?://' AND char_length(webhook_secret) >= 32))
);

CREATE TABLE merchant_payment_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id TEXT NOT NULL REFERENCES merchants(id) ON DELETE RESTRICT,
    merchant_reference TEXT NOT NULL CHECK (char_length(merchant_reference) BETWEEN 1 AND 140),
    amount NUMERIC(19,4) NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    status TEXT NOT NULL DEFAULT 'AWAITING_CUSTOMER'
        CHECK (status IN ('AWAITING_CUSTOMER', 'PROCESSING', 'BOOKED', 'FAILED', 'CANCELLED', 'EXPIRED')),
    payment_order_id UUID UNIQUE REFERENCES payment_orders(id) ON DELETE RESTRICT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (merchant_id, merchant_reference)
);

CREATE INDEX merchant_payment_intents_pending_idx
    ON merchant_payment_intents (expires_at)
    WHERE status = 'AWAITING_CUSTOMER';

-- Local development merchant: not an Identity user and therefore not a
-- login-capable customer. Its account only receives internal ledger credits.
INSERT INTO users (id, email, hashed_password, full_name, role, session_version)
SELECT gen_random_uuid(), 'merchant.pehlione-ecommerce@invalid.local', 'service-managed', 'Pehlione E-Commerce GmbH', 'CUSTOMER', 0
WHERE NOT EXISTS (
    SELECT 1 FROM users WHERE email = 'merchant.pehlione-ecommerce@invalid.local'
);

INSERT INTO accounts (id, owner_id, name, balance, currency, is_system, iban, account_type, status, available_balance)
SELECT gen_random_uuid(), users.id, 'Pehlione E-Commerce Merchant Account', 0, 'EUR', FALSE,
       'DE89999999998000000000', 'SETTLEMENT', 'ACTIVE', 0
FROM users
WHERE users.email = 'merchant.pehlione-ecommerce@invalid.local'
  AND NOT EXISTS (SELECT 1 FROM accounts WHERE iban = 'DE89999999998000000000');

INSERT INTO merchants (id, name, beneficiary_account_id, return_url)
SELECT 'pehlione-ecommerce', 'Pehlione E-Commerce GmbH', accounts.id,
       'http://localhost:8080/checkout/banking/return'
FROM accounts
WHERE accounts.iban = 'DE89999999998000000000'
ON CONFLICT (id) DO NOTHING;
