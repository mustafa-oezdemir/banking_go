# ADR-003: Separate Domain, Application, and Infrastructure Responsibilities

- Status: Accepted
- Date: 2026-09-10
- Deciders: Project owner and architecture roadmap
- Scope: Architecture Phase 2

## Context

ADR-002 established capability ownership inside a modular monolith, but important rules were still mixed with sqlc records, PostgreSQL transactions, and HTTP handlers. That coupling made payment and ledger invariants difficult to test without a database and made framework details appear in application-facing behavior.

Account, Payment, and Ledger must remain one deployable backend and one PostgreSQL consistency boundary. A generic repository for every query or a remote service boundary would add indirection while weakening the existing local ACID model.

## Problem

The code needs useful seams between business policy, use-case orchestration, and mechanisms. The seams must make critical rules independently testable while preserving the serializable transactions and row locks used for financial writes.

## Decision drivers

- Preserve HTTP contracts, database schema, deployment topology, and observable banking behavior.
- Keep debit and credit legs, cached balances, payment state, and audit writes locally atomic.
- Test money, posting, ownership, lifecycle, and idempotency rules without HTTP or PostgreSQL.
- Prevent sqlc and database errors from becoming domain contracts.
- Avoid a mechanical Clean Architecture conversion and low-value one-method abstractions.
- Keep adapters replaceable at boundaries where replacement or focused tests are valuable.

## Options considered

1. Keep business and persistence logic together and rely only on integration tests.
2. Introduce ports and domain types for every query and handler immediately.
3. Extract high-value pure rules and application ports incrementally while retaining the concrete payment unit of work.

## Selected decision

We selected option 3.

### Domain responsibilities

| Package | Responsibility | Infrastructure dependencies |
| --- | --- | --- |
| `internal/ledger/domain` | Account snapshots, transfer policy, balanced posting plans, blocked/system/currency/ownership/insufficient-funds invariants | None |
| `internal/payment/domain` | Typed lifecycle transitions and normalized payment-intent equivalence for idempotent replay | None |
| `internal/account` | Account ownership predicate, profile value normalization, IBAN and account-name rules | None |
| `internal/identity` | Credential policy plus authentication use case and repository port | None |

Money parsing remains in `internal/ledger` because it is a stable Banking Core value boundary shared by ledger commands. It uses exact decimal arithmetic and exposes no persistence type.

### Application responsibilities

- `ledger.Service` accepts application commands, parses exact amounts, creates IDs where needed, plans domain postings, and delegates persistence through the module-owned `ledger.Repository` port.
- `account.ProfileService` owns profile retrieval/update orchestration through `account.ProfileRepository`.
- `identity.AuthenticationService` normalizes credentials, preserves enumeration-resistant failure behavior, verifies password hashes, and reads session versions through `identity.AuthenticationRepository`.
- `payment.Service` orchestrates creation, confirmation, scheduling, and booking. It delegates lifecycle, intent-equivalence, ownership, and posting decisions to pure domain behavior.

Application contracts use UUIDs, decimal values, and module-owned records. They do not expose sqlc rows or `database/sql` errors.

### Infrastructure responsibilities

- `internal/platform/database/LedgerRepository` implements the ledger port with PostgreSQL serializable transactions, deterministic row locking, sqlc mappings, ledger entries, cached balance updates, and audit writes.
- `internal/platform/database/IdentityRepository` and the existing profile adapter map persistence records into module-owned application types.
- `internal/platform/httpapi` decodes/encodes transport DTOs, maps application errors to HTTP status codes, issues JWT cookies, and wires authenticated requester IDs into use cases.
- `cmd` remains the composition root and constructs ports and adapters explicitly.

### Payment unit-of-work exception

`payment.Service` temporarily retains a concrete `*database.Store` dependency. Its confirmation and scheduled-booking flows span payment state, locked account rows, ledger entries, cached balances, and audit events in one serializable transaction. Replacing this with several small repositories would either leak a transaction handle into domain contracts or hide the same concrete coupling behind a broad generic interface.

Instead, Phase 2 extracts the independently valuable rules and keeps the existing transaction implementation intact. A future change may introduce a payment-owned unit-of-work port only when its contract can express the full atomic operation without sqlc leakage or loss of locking semantics.

### Dependency rules

- Core domain packages cannot import HTTP, Chi, sqlc, PostgreSQL drivers, email providers, or process environment packages.
- `ledger` depends on its domain package and account policy, not on `platform/database` or sqlc.
- Infrastructure adapters may depend inward on public module types; modules cannot depend outward on an adapter, except for the documented payment unit-of-work transition.
- Business modules use another module's public behavior only and never access private implementation packages.
- These rules are enforced by `internal/architecture/dependencies_test.go`.

### Transaction boundaries

No runtime or data boundary changes. Account, Payment, and Ledger remain in one binary and database.

- Ledger operations execute entirely inside `LedgerRepository` serializable transactions.
- Payment booking retains its existing serializable transaction across payment state, both ledger legs, both cached balances, and audit state.
- Posting plans are computed before persistence and rechecked against locked account snapshots inside the transaction.
- Notification delivery remains post-commit and cannot affect financial commit outcome.
- Existing idempotency keys and database uniqueness constraints remain authoritative for concurrent duplicate requests.

## Positive consequences

- Critical financial rules run in fast unit tests without HTTP or PostgreSQL.
- Ledger application code no longer imports sqlc, `database/sql`, or the concrete store.
- Login and profile handlers delegate application decisions rather than interpreting database records.
- PostgreSQL mappings and transaction mechanics have a clear adapter owner.
- The architecture fitness test guards pure-domain and module import directions.
- Local ACID behavior and deployment simplicity are preserved.

## Negative consequences

- Payment orchestration still has a documented concrete persistence dependency.
- Account and administration handlers still contain some simple query orchestration.
- Domain and application code share a package in Identity and Account where an extra directory would add little value.
- PostgreSQL integration tests remain necessary to prove locks, retries, constraints, and atomic writes.

## Risks and mitigations

- **Risk:** Pure posting plans could be computed from stale state. **Mitigation:** persistence adapters plan and write using account rows locked inside the same serializable transaction.
- **Risk:** Two representations of payment status drift. **Mitigation:** application constants alias the typed domain statuses and transition tests cover terminal/invalid states.
- **Risk:** Repository ports become generic persistence abstractions. **Mitigation:** ports are owned by a use case and use business-shaped arguments/results only.
- **Risk:** The payment exception becomes permanent accidental coupling. **Mitigation:** it is explicit in this ADR and the architecture import allowlist; expansion requires an ADR update and tests.
- **Risk:** Adapter errors leak sensitive or implementation detail to clients. **Mitigation:** HTTP handlers map stable application errors and return generic 5xx responses for infrastructure failures.

## Revisit conditions

Revisit the payment unit-of-work exception when a new booking workflow, an outbox requirement, or a second persistence adapter provides a concrete reason for a transaction-scoped port. Revisit deployable boundaries only when measured operational requirements outweigh the current local ACID and single-developer simplicity benefits.
