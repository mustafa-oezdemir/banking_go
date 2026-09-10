# ADR-001: Use Evolutionary Microservice Migration

- Status: Accepted
- Date: 2026-09-10
- Deciders: Project owner and architecture roadmap
- Scope: Pehlione DemoBank backend evolution

## Context

Pehlione DemoBank is a learning project maintained by one developer. The current Go backend is deployed as one application and an optional worker over one PostgreSQL schema. It already implements valuable financial behavior: serializable transactions, deterministic account locking, double-entry booking, cached-balance reconciliation, payment state transitions, idempotency, scheduled-payment claims, append-only entries, authorization, and notifications.

The current source packages are primarily technical layers. HTTP handlers, `internal/service`, `internal/db`, sqlc, and email code share concrete database types and responsibilities. This makes boundaries unclear, but it does not mean each folder should become a network service.

Account, Payment, and Ledger collaborate within local PostgreSQL transactions. Splitting them now would replace a working ACID boundary with distributed failure modes and would require sagas or compensating workflows without a business need. At the same time, Notification and eventually Identity may provide useful extraction lessons once their contracts and data ownership are explicit.

## Decision

We will evolve the system incrementally.

1. Establish business-capability boundaries inside the existing Go backend.
2. Move domain policy away from HTTP and PostgreSQL details through module-owned application interfaces and ports.
3. Keep Account, Payment, and Ledger in one Banking Core deployable and local transaction boundary.
4. Extract a module only after it has a clear purpose, stable contract, private data ownership, tested failure semantics, and acceptable operational cost.
5. Treat Notification as the first likely extraction candidate because delivery is post-commit and operationally independent.
6. Consider Identity later, after credentials, sessions, roles, and customer profile ownership are separated explicitly.
7. Use asynchronous messaging only for a concrete durability or decoupling need. Financial booking remains synchronous and idempotent.
8. Record significant boundary, persistence, transport, and consistency decisions in subsequent ADRs.

No big-bang rewrite will be performed.

## Guardrails

- Existing routes, behavior, and schema remain backward compatible whenever reasonably possible.
- `entries` remains append-only and every booking remains balanced.
- Account, Payment, and Ledger changes remain in one serializable database transaction.
- Extracted services never read or write another service's private tables.
- Every service owns its schema/database and communicates through versioned contracts.
- At-least-once event delivery requires durable producer outbox and consumer deduplication.
- Remote infrastructure and providers stay behind explicit adapters.
- A failed phase validation or push stops work before the next phase.

## Consequences

### Positive

- Financial invariants stay protected by existing local ACID transactions.
- Each refactoring step is smaller, reviewable, and reversible.
- Module seams can be tested before network and deployment complexity is introduced.
- Extraction decisions are based on observed coupling and failure needs rather than folder names.
- The architecture remains operable by one developer.

### Negative

- The codebase will temporarily contain compatibility facades and transitional adapters.
- Learning about deployment-level microservices happens later than package restructuring.
- The shared schema remains during early phases, so discipline and architecture tests are required to prevent new cross-module access.
- Some duplication may be preferable to a shared cross-service business library.

### Risks

- A cosmetic folder move could be mistaken for a real boundary.
- Interfaces could become overly generic or mirror sqlc instead of expressing use cases.
- Premature event introduction could obscure transaction semantics.
- Running both API scheduler and worker may remain operationally confusing until deployment profiles are clarified.

These risks are mitigated with module ownership documentation, import rules, architecture tests, contract tests, and phase-by-phase validation.

## Alternatives considered

### Rewrite directly as multiple microservices

Rejected. It would duplicate working behavior, create distributed transactions around money, require immediate service authentication and observability, and carry disproportionate risk for a single-developer learning project.

### Keep the current technical-layer monolith permanently

Rejected as the roadmap direction. It preserves runtime simplicity but does not teach capability boundaries or create safe extraction seams. The system may still remain a modular monolith if later extraction criteria are not met.

### Extract Payment or Ledger first

Rejected. These capabilities currently need the same locks, entries, balances, payment state, and audit records in one transaction. Splitting them has no demonstrated benefit that outweighs distributed consistency cost.

### Extract Notification first without prior modularization

Rejected. The current email adapter reads `users` and `accounts` directly and relies on a non-durable in-memory queue. Extracting it immediately would either share the database or produce an incomplete delivery contract.

## Validation

This decision is supported by the Phase 0 current-state dependency map, transaction inventory, and risk analysis in:

- `docs/architecture/current-architecture.md`
- `docs/architecture/target-architecture.md`
- `docs/architecture/migration-roadmap.md`

The decision should be revisited if real scaling, team ownership, regulatory isolation, availability, or independent-release requirements emerge. A request for more services by itself is not sufficient evidence.
