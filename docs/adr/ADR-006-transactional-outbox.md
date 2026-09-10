# ADR-006: Transactional outbox for Banking events

- Status: Accepted
- Date: 2026-09-11
- Scope: Architecture Phase 4

## Context

Publishing to RabbitMQ inside a payment request before the PostgreSQL transaction commits can expose an event for a payment that subsequently rolls back. Publishing after commit without a durable record can lose an event if the process crashes between the two writes.

## Decision

Payment booking and terminal payment failure insert an `outbox_events` row in the same serializable PostgreSQL transaction that writes payment state, ledger entries, balances, and audit rows. The transaction commits first. A separate publisher leases unpublished rows with `FOR UPDATE SKIP LOCKED`, publishes the envelope with RabbitMQ confirms, then marks the row published.

The outbox lease expires after a publisher crash. The next publisher can publish the same event again. This is intentional at-least-once behavior. `publish_attempts`, `lease_until`, and a bounded diagnostic support operations without making delivery error text a source of sensitive data.

## Transaction boundary

```text
BEGIN
  payment state + ledger postings + balances
  INSERT outbox_events
COMMIT
  outbox publisher -> RabbitMQ -> Notification
```

No RabbitMQ call is made from the database transaction. A successful financial commit is never rolled back because the broker is unavailable.

## Consequences

The database is the source of truth for events awaiting publication. This adds polling and cleanup considerations, but prevents the dual-write inconsistency of direct database-plus-broker updates.
