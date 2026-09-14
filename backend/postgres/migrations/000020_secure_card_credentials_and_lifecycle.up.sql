-- Banking may reveal virtual-card credentials to their authenticated owner.
-- Credentials remain encrypted at rest; CVC is never returned by list APIs.
ALTER TABLE payment_cards
    ADD COLUMN encrypted_pan BYTEA,
    ADD COLUMN encrypted_cvc BYTEA;

ALTER TABLE payment_cards DROP CONSTRAINT IF EXISTS payment_cards_credential_version_check;
ALTER TABLE payment_cards
    ADD CONSTRAINT payment_cards_credential_version_check
        CHECK (credential_version IN (0, 1, 2));

ALTER TABLE payment_cards DROP CONSTRAINT IF EXISTS payment_cards_status_check;
ALTER TABLE payment_cards
    ADD CONSTRAINT payment_cards_status_check
        CHECK (status IN ('ACTIVE', 'BLOCKED', 'EXPIRED', 'CANCELLED'));

ALTER TABLE merchant_card_tokens
    DROP CONSTRAINT IF EXISTS merchant_card_tokens_card_id_fkey;
ALTER TABLE merchant_card_tokens
    ADD CONSTRAINT merchant_card_tokens_card_id_fkey
        FOREIGN KEY (card_id) REFERENCES payment_cards(id) ON DELETE CASCADE;
