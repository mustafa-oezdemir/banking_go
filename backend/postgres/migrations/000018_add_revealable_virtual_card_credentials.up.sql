-- Version 1 cards derive their demo credentials from CARD_DATA_KEY and the card
-- UUID. Legacy cards remain unrevealable because their random credentials were
-- deliberately never persisted.
ALTER TABLE payment_cards
    ADD COLUMN credential_version SMALLINT NOT NULL DEFAULT 0
    CHECK (credential_version IN (0, 1));
