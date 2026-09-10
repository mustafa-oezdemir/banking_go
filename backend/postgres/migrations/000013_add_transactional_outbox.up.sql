-- Transactional outbox for Banking-owned events. A row is written in the
-- same PostgreSQL transaction as the payment and ledger mutations, then a
-- separate publisher forwards it to RabbitMQ after commit.
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL CHECK (event_type IN ('payment.booked.v1', 'payment.failed.v1')),
    event_version INTEGER NOT NULL CHECK (event_version = 1),
    aggregate_id UUID NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
    correlation_id TEXT NOT NULL CHECK (char_length(correlation_id) BETWEEN 1 AND 128),
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    lease_until TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX outbox_events_publishable_idx
    ON outbox_events (created_at)
    WHERE published_at IS NULL;

-- Notification owns this schema and table. It never queries Banking users,
-- accounts, payments, or ledger tables; this table only de-duplicates event IDs.
CREATE SCHEMA IF NOT EXISTS notification;

CREATE TABLE notification.processed_events (
    event_id UUID PRIMARY KEY,
    processing_until TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 1 CHECK (attempts > 0),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX notification_processed_events_lease_idx
    ON notification.processed_events (processing_until)
    WHERE processed_at IS NULL;
