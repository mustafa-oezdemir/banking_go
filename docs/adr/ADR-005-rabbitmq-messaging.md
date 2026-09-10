# ADR-005: RabbitMQ for committed payment notification events

- Status: Accepted
- Date: 2026-09-11
- Scope: Architecture Phase 4

## Decision

RabbitMQ is the asynchronous transport between Banking's committed payment events and Notification. Banking publishes durable JSON event envelopes to the durable topic exchange `banking.events`; Notification consumes from the durable `notification.payment.v1` queue.

The only current routing keys are `payment.booked.v1` and `payment.failed.v1`. Both have an actual Notification email consumer. The event envelope contains `event_id`, `event_type`, `event_version`, `occurred_at`, `aggregate_id`, `correlation_id`, and a fully resolved activity-email payload.

The publisher uses persistent messages and RabbitMQ publisher confirms. A missing confirmation is a failed publication: its outbox row stays eligible for later delivery.

## Topology

- `banking.events` is the primary durable topic exchange.
- `notification.payment.v1` binds `payment.*.v1` and is consumed with manual acknowledgements.
- Transient failures are republished to `banking.events.retry`; `notification.payment.retry` holds them for the configured TTL and dead-letters them back to the primary exchange.
- Invalid payloads and exhausted retry attempts are published to `banking.events.dlx`, routing key `notification.payment.dead`, and retained in `notification.payment.dlq` for inspection.

## Consequences

RabbitMQ unavailability does not block or roll back a financial commit. It delays notification delivery until the outbox publisher reconnects. RabbitMQ is not treated as an authoritative banking ledger or an exactly-once system.
