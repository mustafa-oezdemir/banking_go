-- Repair databases whose migration history was advanced before the role
-- column became part of migration 000006. Fresh databases already satisfy
-- this migration, so every statement remains idempotent.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'CUSTOMER';

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('CUSTOMER', 'ADMIN'));

CREATE INDEX IF NOT EXISTS users_role_idx ON users (role);
