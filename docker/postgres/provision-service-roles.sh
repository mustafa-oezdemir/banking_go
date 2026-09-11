#!/bin/sh
set -eu

# This runs as the PostgreSQL administrator before migrations. It is
# idempotent so an existing local volume receives the same runtime roles.
: "${DB_HOST:?DB_HOST is required}"
: "${DB_NAME:?DB_NAME is required}"
: "${DB_USER:?DB_USER is required}"
: "${DB_PASSWORD:?DB_PASSWORD is required}"
: "${BANKING_DB_PASSWORD:?BANKING_DB_PASSWORD is required}"
: "${IDENTITY_DB_PASSWORD:?IDENTITY_DB_PASSWORD is required}"
: "${NOTIFICATION_DB_PASSWORD:?NOTIFICATION_DB_PASSWORD is required}"

export PGPASSWORD="$DB_PASSWORD"

psql \
  -h "$DB_HOST" \
  -p "${DB_PORT:-5432}" \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  -v ON_ERROR_STOP=1 \
  -v banking_password="$BANKING_DB_PASSWORD" \
  -v identity_password="$IDENTITY_DB_PASSWORD" \
  -v notification_password="$NOTIFICATION_DB_PASSWORD" <<'SQL'
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'banking_app') THEN
        CREATE ROLE banking_app LOGIN NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'identity_app') THEN
        CREATE ROLE identity_app LOGIN NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'notification_app') THEN
        CREATE ROLE notification_app LOGIN NOINHERIT;
    END IF;
END $$;

SELECT format('ALTER ROLE banking_app LOGIN PASSWORD %L', :'banking_password') \gexec
SELECT format('ALTER ROLE identity_app LOGIN PASSWORD %L', :'identity_password') \gexec
SELECT format('ALTER ROLE notification_app LOGIN PASSWORD %L', :'notification_password') \gexec
SQL
