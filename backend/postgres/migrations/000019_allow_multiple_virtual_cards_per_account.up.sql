DROP INDEX IF EXISTS payment_cards_active_account_idx;

CREATE INDEX IF NOT EXISTS payment_cards_account_idx
    ON payment_cards (account_id, created_at DESC);
