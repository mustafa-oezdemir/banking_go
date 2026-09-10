# ADR-002: Establish Domain Module Boundaries

- Status: Accepted
- Date: 2026-09-10
- Deciders: Project owner and architecture roadmap
- Scope: Phase 1 backend modularization

Application/infrastructure separation within these boundaries is refined by [ADR-003](ADR-003-domain-infrastructure-separation.md).

## Context

The backend began as a package-by-technical-layer monolith: HTTP behavior lived in `internal/api`, banking and bootstrap behavior shared `internal/service`, persistence helpers lived in `internal/db`, and provider adapters lived beside business code. The deployment and PostgreSQL transaction model are appropriate for the project, but the source layout did not communicate ownership and made accidental coupling easy.

Account, Payment, and Ledger collaborate inside serializable PostgreSQL transactions. Extracting them into separate processes now would weaken a working ACID boundary and introduce distributed transaction failure modes without an operational requirement.

## Problem

The code needs explicit capability ownership and enforceable dependency direction before application/persistence ports or deployable services can be introduced. A folder-only reorganization is insufficient: allowed dependencies, forbidden dependencies, and transaction ownership must be stated and tested.

## Decision drivers

- Preserve all existing routes, schemas, financial invariants, and Docker topology.
- Keep monetary booking locally atomic and idempotent.
- Make module ownership visible to maintainers and static checks.
- Prevent HTTP and provider concerns from entering business modules.
- Allow a staged removal of existing sqlc and concrete-store coupling in Phase 2.
- Keep the architecture manageable for one developer.

## Options considered

1. Keep the technical-layer packages unchanged.
2. Split Account, Payment, and Ledger into separate services immediately.
3. Establish domain-oriented packages inside the existing deployable and enforce their dependencies.

## Selected decision

We selected option 3. The backend remains one deployable banking application, with an optional worker over the same code and database, and is organized into these modules:

| Module | Owns | Application-facing behavior |
| --- | --- | --- |
| `internal/identity` | Email normalization, password policy, hashing, verification, and timing-safe dummy verification | Credential-policy functions used by HTTP and bootstrap adapters |
| `internal/account` | IBAN rules and customer profile/name invariants | IBAN generation/validation/masking and normalized profile input |
| `internal/ledger` | EUR money parsing, account balance rules, double-entry posting, reconciliation, and local transfers | `ledger.Service` operations and stable financial errors |
| `internal/payment` | VoP, payment intent/idempotency, payment state transitions, booking, scheduled processing, standing orders, and owner-scoped event signals | `payment.Service`, payment commands/results, worker operations, and `EventHub` |
| `internal/notification` | Provider-neutral notification message contracts | `Sender` port and no-op implementation |
| `internal/platform/httpapi` | Chi routing, JWT/cookie/session middleware, CSRF/CORS/security headers, HTTP DTOs, mapping, and response semantics | HTTP adapter only |
| `internal/platform/database` | Concrete PostgreSQL store, serializable unit of work, retries, profile/reset/admin persistence | Infrastructure adapter used during composition and the Phase 1 transition |
| `internal/platform/email` | SMTP and Resend delivery | `notification.Sender` adapters |
| `internal/platform/bootstrap` | Optional demo/admin startup seeding | Startup adapter called by `cmd/main` |
| `cmd` | Configuration, dependency composition, process lifecycle, route wiring, and scheduler startup | Executable entry points |

### Allowed dependencies

- `identity`, `account`, and `notification` do not import another project module.
- `ledger` may import `account` plus the transitional PostgreSQL adapter and generated sqlc records.
- `payment` may import `account`, `ledger`, and the `notification` port plus the same transitional persistence packages.
- Platform adapters may import business modules and their public application-facing behavior.
- `cmd` is the composition root and may construct all modules.

The exact Phase 1 import allowlist is executable in `backend/internal/architecture/dependencies_test.go`.

### Forbidden dependencies

- Business modules must not import `platform/httpapi`, `platform/email`, Chi, `net/http`, SMTP, or the Resend SDK.
- `identity`, `account`, and `notification` must not import PostgreSQL, sqlc, or the concrete store.
- Platform/database and provider adapters must not become generic shared business-utility packages.
- Modules must not reach into another module's private implementation or bypass its public behavior.
- No new deployable or network boundary is created between Account, Payment, and Ledger in this phase.

### Transaction boundaries

Account, Payment, and Ledger remain one Banking Core consistency boundary. The existing `platform/database.Store.ExecTx` serializable unit of work continues to own local atomicity for:

- account creation and opening entries;
- deposits, withdrawals, and same-owner transfers;
- payment confirmation, ledger posting, cached balances, payment state, and audit writes;
- scheduled payment claiming and per-payment booking;
- password reset token consumption, password replacement, and session revocation;
- administrator changes and their audit record.

Notification delivery remains post-commit and cannot roll back an already committed financial operation. The API and optional worker share the same schema and transaction implementation.

## Positive consequences

- Source ownership now mirrors business capabilities without adding runtime complexity.
- Identity and Account rules can be tested without HTTP or PostgreSQL.
- HTTP, email, database, and bootstrap mechanisms are visibly platform concerns.
- An automated fitness function prevents forbidden imports and accidental boundary erosion.
- Existing financial transactions, idempotency, and deployment behavior remain intact.

## Negative consequences

- `ledger` and `payment` still depend on the concrete store, sqlc records, and `database/sql`.
- HTTP handlers still perform some application orchestration and persistence mapping.
- Test fixtures are duplicated across package-level PostgreSQL integration tests.
- The shared schema does not enforce data ownership by itself.

## Risks and mitigations

- **Risk:** Package moves may be mistaken for full domain/infrastructure separation. **Mitigation:** The remaining persistence coupling is explicitly transitional and is a Phase 2 exit criterion.
- **Risk:** New imports can recreate technical-layer coupling. **Mitigation:** The architecture dependency test runs with the normal Go test suite.
- **Risk:** Splitting business packages could obscure transaction ownership. **Mitigation:** Account, Payment, and Ledger remain one deployable and share the existing serializable unit of work.
- **Risk:** Provider failure could affect money state. **Mitigation:** Notification delivery remains post-commit and provider-neutral at the business boundary.

## Revisit conditions

Revisit this decision when Phase 2 replaces concrete persistence dependencies with narrow module-owned ports, or when measured team, scaling, regulatory, or availability requirements justify a deployable boundary. Service extraction alone is not a reason to weaken the local financial transaction.
