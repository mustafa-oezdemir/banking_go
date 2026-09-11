-- Identity owns this schema. Banking never reads it; its customer table only
-- maps the authenticated subject UUID to account ownership.
CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE IF NOT EXISTS identity.users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    full_name TEXT NOT NULL,
    hashed_password TEXT NOT NULL,
    session_version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS identity.password_reset_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Preserve existing local demo identities while the former Banking credential
-- column is phased out. The Identity service becomes the only runtime reader.
INSERT INTO identity.users (id, email, full_name, hashed_password, session_version, created_at, updated_at)
SELECT id, email, full_name, hashed_password, session_version, created_at, CURRENT_TIMESTAMP
FROM public.users
ON CONFLICT (id) DO NOTHING;
