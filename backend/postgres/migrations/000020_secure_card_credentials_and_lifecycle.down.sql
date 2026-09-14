UPDATE payment_cards SET status = 'BLOCKED' WHERE status = 'CANCELLED';

ALTER TABLE merchant_card_tokens
    DROP CONSTRAINT IF EXISTS merchant_card_tokens_card_id_fkey;
ALTER TABLE merchant_card_tokens
    ADD CONSTRAINT merchant_card_tokens_card_id_fkey
        FOREIGN KEY (card_id) REFERENCES payment_cards(id) ON DELETE RESTRICT;

ALTER TABLE payment_cards DROP CONSTRAINT IF EXISTS payment_cards_status_check;
ALTER TABLE payment_cards
    ADD CONSTRAINT payment_cards_status_check
        CHECK (status IN ('ACTIVE', 'BLOCKED', 'EXPIRED'));

UPDATE payment_cards SET credential_version = 0 WHERE credential_version = 2;
ALTER TABLE payment_cards DROP CONSTRAINT IF EXISTS payment_cards_credential_version_check;
ALTER TABLE payment_cards
    ADD CONSTRAINT payment_cards_credential_version_check
        CHECK (credential_version IN (0, 1));

ALTER TABLE payment_cards
    DROP COLUMN encrypted_pan,
    DROP COLUMN encrypted_cvc;
