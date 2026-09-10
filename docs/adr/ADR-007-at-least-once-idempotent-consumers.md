# ADR-007: At-least-once delivery with idempotent Notification consumers

- Status: Accepted
- Date: 2026-09-11
- Scope: Architecture Phase 4

## Decision

Notification persists event IDs in its owned `notification.processed_events` table before acknowledging RabbitMQ delivery. An event is claimed with a lease, delivered, marked processed, and only then ACKed. A previously processed `event_id` is ACKed without a second provider call.

The table is owned by Notification even though local Compose uses the same PostgreSQL instance. The service has no query path to Banking users, accounts, payment orders, or ledger tables.

Transient provider, store, or broker failures are retried through a delayed retry queue. Retries stop after `RABBITMQ_MAX_RETRIES`; malformed, unsupported, or exhausted messages are sent to a durable dead-letter queue. A leased in-progress event is not ACKed as a duplicate; it is retried, so a crashed consumer cannot silently lose it.

## Guarantees and non-guarantees

- Banking event publication and consumer processing are **at least once**.
- A duplicate broker delivery after successful processing produces no second email in the normal path.
- A crash after a provider accepts an email but before `processed_at` is stored can still cause a duplicate email on retry. This external side effect cannot be atomically committed with PostgreSQL and SMTP/Resend.
- The system does **not** claim exactly-once delivery.

## Tests

The contract tests validate envelope schema; consumer tests cover duplicate suppression and transient failure release. The Compose smoke validation republishes the exact same event ID and confirms MailHog receives no second message. Broker-down recovery verifies that an outbox row remains unpublished during downtime and is delivered after RabbitMQ returns.
