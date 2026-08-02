ALTER TABLE entries
    DROP COLUMN IF EXISTS execution_date,
    DROP COLUMN IF EXISTS booking_date,
    DROP COLUMN IF EXISTS category,
    DROP COLUMN IF EXISTS purpose,
    DROP COLUMN IF EXISTS counterparty_iban,
    DROP COLUMN IF EXISTS counterparty_name,
    DROP COLUMN IF EXISTS payment_order_id;

DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS payment_orders;
DROP TABLE IF EXISTS standing_orders;
DROP TABLE IF EXISTS beneficiaries;

DROP INDEX IF EXISTS accounts_owner_status_idx;
DROP INDEX IF EXISTS accounts_iban_unique_idx;

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_status_check,
    DROP CONSTRAINT IF EXISTS accounts_account_type_check,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS available_balance,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS account_type,
    DROP COLUMN IF EXISTS iban;

ALTER TABLE users DROP COLUMN IF EXISTS full_name;
