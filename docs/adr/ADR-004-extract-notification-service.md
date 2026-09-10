# ADR-004: Extract the Notification Service

- Status: Accepted
- Date: 2026-09-11
- Deciders: Project owner and architecture roadmap
- Scope: Architecture Phase 3

## Context

Email delivery is outside the Banking Core financial transaction. Before this decision, the Banking API hosted the SMTP/Resend adapter and the adapter queried private `users` and `accounts` tables after financial commits. Provider failure did not roll back money, but delivery ownership and private-data access prevented Notification from becoming an independent deployable.

## Problem

The first service extraction must create a real process and ownership boundary without weakening Account, Payment, and Ledger atomicity. Notification needs an explicit contract, independent health/configuration, bounded failures, and enough recipient/template data to operate without Banking database access.

## Decision drivers

- Financial commits must never depend on SMTP, Resend, or Notification availability.
- Notification must not connect to PostgreSQL or query Banking-owned tables.
- Local development must continue to deliver through MailHog.
- The boundary must carry correlation IDs and use structured logs.
- Network waits must be bounded and client-visible errors must not expose provider details.
- Phase 3 should not introduce a broker or pretend to provide durable/exactly-once delivery.

## Options considered

1. Keep email inside the Banking API.
2. Extract Notification and call it synchronously over private HTTP.
3. Add RabbitMQ immediately without a transactional outbox.

## Selected decision

We selected option 2 for Phase 3.

Docker Compose now runs `frontend`, `banking-api`, `notification-service`, `postgres`, and `mailhog`. The Notification executable is `backend/cmd/notification-service` and has its own non-root image, port, provider configuration, delivery timeout, structured logger, graceful shutdown, and `/health` endpoint.

### Private HTTP contract

The Banking API calls:

- `POST /v1/notifications/password-reset`
- `POST /v1/notifications/account-activity`

Successful delivery returns `204 No Content`. Commands use strict JSON decoding, a 64 KiB body limit, bounded fields, validated email/amount data, and a shared service token in the `Authorization: Bearer` header. `/health` is unauthenticated. `X-Request-ID` is forwarded or generated and returned by the service.

### Data ownership

The Banking-side adapter resolves the user and account after commit and sends an explicit command containing recipient email/name, account display name, masked IBAN, balance, amount, direction, and optional reference/counterparty. Notification receives no user/account IDs that require private-table lookup and has no database dependency.

Password reset commands contain the recipient, display name, and opaque one-time token needed to construct the configured frontend URL. Tokens and recipient data are never included in logs.

### Failure and delivery semantics

- Financial operations commit before an activity command is attempted.
- Notification failure is logged as a post-commit warning and never changes the banking response or booked state.
- Password-reset token persistence also precedes delivery; the public endpoint keeps its enumeration-resistant accepted response.
- The Banking client has a four-second HTTP timeout; the service has a twelve-second provider timeout and bounded HTTP server timeouts.
- Phase 3 performs one HTTP attempt only. Automatic retries are intentionally absent because SMTP/Resend may have accepted a request before a network failure becomes visible. Retrying without durable idempotency could duplicate email.
- Delivery is therefore best effort in Phase 3. It is not exactly once and not guaranteed during downtime.

## Positive consequences

- Notification is a separately runnable, health-checkable container.
- SMTP/Resend credentials and provider libraries leave the Banking runtime configuration.
- Notification cannot read Banking private tables; the boundary is explicit and testable.
- Provider failure cannot roll back or corrupt a committed financial transaction.
- MailHog remains the default local provider.
- API contract, authentication, correlation, failure, and no-retry behavior have focused tests.

## Negative consequences

- A synchronous call adds up to the bounded client timeout after a commit.
- An outage can lose best-effort activity notifications.
- The shared bearer token requires coordinated rotation and is not a full service-identity system.
- Resolving recipient/account display data adds post-commit Banking reads.

## Risks and mitigations

- **Risk:** A command is delivered but the response is lost. **Mitigation:** no blind retry in Phase 3; Phase 4 introduces stable event IDs and consumer deduplication.
- **Risk:** Private data leaks through logs. **Mitigation:** logs contain request ID, event kind, status, and errors but never command bodies, addresses, tokens, full IBANs, or balances.
- **Risk:** Callers bypass the private contract. **Mitigation:** a minimum-length shared token is required and compared in constant time.
- **Risk:** Notification accidentally gains Banking persistence access. **Mitigation:** architecture tests reject database/sql, PostgreSQL/sqlc, and Banking database-adapter imports from the executable.
- **Risk:** Notification downtime delays API responses. **Mitigation:** the Banking client timeout is shorter than the API write timeout and failure is handled post-commit.

## Revisit conditions

Phase 4 may replace activity HTTP commands with transactional-outbox events and an idempotent RabbitMQ consumer. Password-reset commands may remain synchronous until a durable security-message design is explicitly chosen. Exactly-once delivery must not be promised.
