-- Virtual cards belong to Banking accounts. Raw PAN and CVC values are never
-- persisted: Banking stores keyed fingerprints and slow password hashes only.
CREATE TABLE payment_cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    pan_fingerprint BYTEA NOT NULL UNIQUE,
    last_four CHAR(4) NOT NULL CHECK (last_four ~ '^[0-9]{4}$'),
    brand TEXT NOT NULL CHECK (brand IN ('visa')),
    exp_month SMALLINT NOT NULL CHECK (exp_month BETWEEN 1 AND 12),
    exp_year SMALLINT NOT NULL CHECK (exp_year BETWEEN 2026 AND 2200),
    cvc_hash TEXT NOT NULL CHECK (char_length(cvc_hash) >= 40),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'BLOCKED', 'EXPIRED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX payment_cards_owner_idx ON payment_cards (owner_id, created_at DESC);
CREATE UNIQUE INDEX payment_cards_active_account_idx ON payment_cards (account_id) WHERE status = 'ACTIVE';

CREATE TABLE merchant_card_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id TEXT NOT NULL REFERENCES merchants(id) ON DELETE RESTRICT,
    card_id UUID NOT NULL REFERENCES payment_cards(id) ON DELETE RESTRICT,
    token_fingerprint BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    UNIQUE (merchant_id, card_id)
);

CREATE INDEX merchant_card_tokens_card_idx ON merchant_card_tokens (card_id);
