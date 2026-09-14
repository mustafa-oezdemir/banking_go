DROP INDEX IF EXISTS payment_cards_account_idx;

-- This rollback intentionally fails when an account already owns multiple
-- active cards; those cards must be deactivated explicitly before downgrading.
CREATE UNIQUE INDEX payment_cards_active_account_idx
    ON payment_cards (account_id)
    WHERE status = 'ACTIVE';
