# Distributed fitness functions

Status: Architecture Phase 9 completed

Last verified: 2026-09-11

This document turns the distributed design into verifiable properties. A scenario is successful only if its stated financial boundary remains true; a green HTTP response alone is not sufficient.

| Scenario | Automated evidence | Expected behavior and invariant |
| --- | --- | --- |
| Notification unavailable | `platform/notificationclient` no-retry test; payment outbox tests | Password-reset command returns a bounded error once. A booked/failed payment still commits its ledger state and outbox row because email delivery is post-commit. |
| RabbitMQ unavailable then recovers | `outbox.Runner` lease/release implementation; Compose verification in `runbooks/rabbitmq-down.md` | Publisher confirmation failure leaves `published_at` empty; a later publisher run retries the committed event. No booking rolls back. |
| Duplicate broker delivery | `platform/rabbitmq:TestProcessorIgnoresDuplicateDelivery` | The Notification inbox claim permits exactly one provider invocation for the same event ID. Delivery semantics remain at-least-once, never exactly-once. |
| Outbox publisher crash/restart | expired outbox lease query and runner contract | An unconfirmed or leased event becomes claimable after `lease_until`; it can be published again and must remain consumer-idempotent. |
| Identity unavailable | `platform/gateway:TestGatewayIdentityOutageDoesNotBlockBankingRoute` | Identity routes fail as `502`; an already-authenticated Banking route still reaches Banking. Authentication changes remain unavailable until Identity recovers. |
| PostgreSQL unavailable | all application service health checks; `runbooks/postgres-down.md` | Requests needing persistence fail closed. No payment is reported booked without its ledger transaction and outbox row. |
| Same idempotency key | `payment:TestScheduledExternalPaymentIsIdempotentAndBalanced` | The same normalized intent returns the original payment; booking happens once and creates balanced ledger legs. |
| Conflicting idempotency key | `payment:TestSamePaymentIntentRejectsIdempotencyPayloadChange` | A changed payment intent is rejected with `ErrIdempotencyConflict`; it never overwrites the original request. |
| Malformed event | `notification` event contract test and `rabbitmq:TestProcessorDeadLettersMalformedEventWithoutCallingProvider` | Unsupported or malformed events do not call the provider and enter the dead-letter path. |

## Mandatory finance fitness function

For every successful payment booking, `SUM(debit) = SUM(credit)` for the payment's `ledger_transaction_id`, each leg is positive on exactly one side, and cached balances are changed in the same serializable PostgreSQL transaction. The integration test `payment:TestScheduledExternalPaymentIsIdempotentAndBalanced` exercises concurrent scheduled workers and verifies both ledger legs, cached balance and a single event.

## Manual Compose recovery checks

Use disposable demo data only. These checks intentionally alter running containers, so they are not CI steps:

1. Stop `rabbitmq`, book a payment, and confirm the payment plus an unpublished outbox row exist.
2. Start `rabbitmq` and wait for the outbox backlog to return to zero. Verify the Notification consumer has one processed event ID and the ledger entries did not change during recovery.
3. Stop `notification-service`, publish an already-committed event, restart the service, and check it is delivered once. Replaying the same event ID must not cause another provider call.
4. Stop `identity-service`; `/login` must return `502` from Gateway, while a request using a valid existing access token can still reach Banking until its token/session validation requires unavailable state.
5. Stop `postgres`; persistence-bound services must become unhealthy/fail closed. Restart PostgreSQL and verify migrations, role provisioner and application health recover without manual schema edits.

The safe command sequence and investigation steps live in the existing runbooks for RabbitMQ, PostgreSQL, Notification and outbox backlog. Never inspect email bodies, tokens, full IBANs, or payment payloads in logs/metrics while diagnosing these scenarios.
