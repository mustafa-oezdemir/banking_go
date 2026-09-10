# Evolutionary Migration Roadmap

Status: Phase 1 completed; Phase 2 awaits explicit start

Last updated: 2026-09-10

## Working agreement

Only one phase is active at a time. Every phase ends with documentation updates, relevant tests and static/security checks, affected builds, Docker validation, diff review, a meaningful commit, and a successful push. A failed push stops the roadmap.

Backward compatibility is preferred. Financial behavior is changed only with explicit tests and an ADR. Account, Payment, and Ledger remain one deployable unit until a documented requirement outweighs the loss of local ACID transactions.

## Phase overview

| Phase | Outcome | Production topology change |
| --- | --- | --- |
| 0. Baseline analysis | Completed: current/target architecture, risks, invariants, and migration decision documented | None |
| 1. Establish domain boundaries | Completed: explicit Identity, Customer/Account, Payment, Ledger, Notification, and Platform modules with an import fitness test | None |
| 2. Modularize banking backend | HTTP and PostgreSQL become adapters around module application interfaces | None |
| 3. Extract notification service | Notification owns delivery and its private persistence | One new optional service |
| 4. Introduce async notification events | Durable outbox, broker, inbox/deduplication, retries | Broker only if justified |
| 5. Extract identity service | Credentials and sessions move behind an Identity contract and private database | One new service |
| 6. Introduce gateway and service auth | Stable external routing and authenticated internal calls | Gateway only if needed |
| 7. Add distributed observability | Cross-service logs, metrics, traces, and SLO-oriented dashboards | Telemetry components as needed |
| 8. Harden architecture and contracts | Compatibility, resilience, security, recovery, and operations gates | No required new service |

## Phase 0 — Baseline analysis

Goal: understand and document the system without changing production behavior.

Deliverables:

- `current-architecture.md` with C4-style context/container/component views;
- `target-architecture.md` with conditional extraction target;
- this migration roadmap;
- `ADR-001-evolutionary-microservice-migration.md`;
- verified baseline tests, builds, security checks, and Compose health.

Exit criteria:

- financial invariants, transaction boundaries, idempotency, shared data, and coupling hotspots are recorded;
- no production code changes are part of the phase;
- baseline commit is pushed.

## Phase 1 — Establish domain boundaries

Goal: organize the existing backend by business capability while retaining one executable and database.

Planned work:

1. Inventory each public type and operation and assign one domain owner.
2. Introduce modules for Identity, Customer/Account, Payment, Ledger, Notification, and Platform only where existing responsibilities support them.
3. Move pure rules first: money, account state, payment state transitions, identity policy, and profile validation.
4. Define application-facing commands/queries and module-owned interfaces.
5. Keep HTTP DTOs and sqlc records outside domain APIs.
6. Add architecture tests that reject forbidden imports.
7. Create `ADR-002-domain-module-boundaries.md` with ownership, allowed dependencies, forbidden dependencies, and transaction rules.

Compatibility strategy:

- preserve routes, JSON shapes, database schema, and Compose topology;
- use temporary facades where necessary so moves are incremental;
- retain Account + Payment + Ledger in one process and unit of work.

Exit criteria:

- modules compile independently according to documented import rules;
- no handler can import another module's internal implementation;
- existing API/integration tests and race tests pass;
- backend, frontend, Mage Linux binary, and Docker stack pass.

## Phase 2 — Modularize banking backend

Goal: complete the ports-and-adapters separation needed for safe extraction.

Planned work:

- replace concrete `*db.Store` dependencies in application services with narrow module-owned repository ports;
- map sqlc rows to domain/application types at PostgreSQL adapter boundaries;
- reduce `cmd/main.go` to composition and lifecycle management;
- move account, beneficiary, profile, and identity orchestration out of handlers;
- define a Banking Core unit-of-work abstraction that preserves existing serializable transactions;
- make clocks and ID generation injectable where determinism matters;
- add repository integration and application-level fake tests;
- decide and document how lifecycle state plus audit/outbox writes become atomic.

Exit criteria:

- domain packages do not import HTTP, Chi, sqlc, PostgreSQL, SMTP/Resend, or process environment packages;
- HTTP and PostgreSQL are replaceable adapters;
- no topology or data ownership change yet.

## Phase 3 — Extract notification service

Goal: isolate a failure-tolerant operational capability without risking financial consistency.

Prerequisites:

- Banking Core emits provider-neutral notification intents after commit;
- notification payloads contain the minimum required recipient/template data so the new service never queries banking tables;
- delivery deduplication and observable retry behavior are designed.

Incremental path:

1. Run Notification as an in-process module behind a transport-neutral port.
2. Add a separate executable and private notification persistence.
3. Support dual-run or shadow delivery with delivery disabled on one side.
4. Switch delivery ownership behind configuration.
5. Remove direct database access from the old email adapter.

Do not add a broker in this phase unless the durable delivery requirements cannot be met by a simpler database-backed handoff.

Rollback: route intents back to the in-process adapter while retaining deduplication IDs and the notification database.

## Phase 4 — Introduce asynchronous notification events

Goal: guarantee that committed banking facts can be delivered at least once without coupling transactions to provider availability.

Planned work:

- add a transactional outbox to Banking Core;
- publish versioned events through an adapter;
- introduce a broker only after choosing it in an ADR from measured needs;
- add a Notification inbox/unique event constraint;
- bound retries and provide dead-letter inspection/replay;
- test duplicate, reordered, delayed, and unavailable-consumer cases.

Exit criteria:

- a committed financial transaction cannot lose its notification event;
- duplicate delivery cannot duplicate notification side effects;
- broker outage never rolls back or blocks ledger booking.

## Phase 5 — Extract identity service

Goal: move credentials, login/session policy, and password reset behind an independently owned service and database.

Required decisions:

- whether profile/contact data remains Customer-owned or moves with Identity;
- role authority and propagation model;
- token issuer/key rotation and internal authorization model;
- migration of existing password hashes, reset tokens, and session versions;
- behavior during Identity unavailability.

Incremental path:

1. Introduce an in-process Identity application interface.
2. Add contract tests and a separate Identity executable/database.
3. Migrate data with reconciliation and rollback scripts.
4. Route login/reset operations to Identity.
5. Replace Banking Core user foreign-key assumptions with stable external identity IDs where needed.

Do not let Identity read Banking Core tables or Banking Core read Identity tables.

## Phase 6 — Introduce gateway and service authentication

Goal: keep browser contracts stable as internal deployables appear.

Use the current Next.js rewrite layer as the starting point. Introduce a dedicated gateway only if routing, policy enforcement, or operational ownership outgrows the web/BFF.

Planned work when justified:

- stable external routes and version policy;
- explicit browser-session termination point;
- service identities and short-lived credentials;
- authorization propagation with documented freshness;
- remote-call timeout, retry, circuit-breaker, and idempotency policy;
- no direct public exposure of internal services.

## Phase 7 — Distributed observability

Goal: make a multi-process request and asynchronous event traceable end to end.

Planned work:

- common correlation/causation propagation;
- service/version fields in structured logs;
- request latency/error, transaction retry, scheduler, outbox, consumer, and delivery metrics;
- OpenTelemetry tracing when cross-service troubleshooting warrants it;
- separate liveness/readiness probes;
- dashboards and alerts focused on user-visible failure and backlog, not infrastructure noise.

PII, secrets, JWTs, raw tokens, and unmasked financial identifiers remain excluded from telemetry.

## Phase 8 — Harden architecture and contracts

Goal: prove that the evolved system is maintainable and failure-safe.

Planned work:

- OpenAPI compatibility checks and consumer-driven/event contract tests;
- schema migration compatibility and rollback rehearsals;
- backup/restore and disaster-recovery exercises;
- load/concurrency tests for idempotency and booking;
- broker/SMTP/database outage tests;
- dependency, image, SBOM, provenance, secret, and CodeQL gates;
- documented runbooks for stuck payments, outbox lag, duplicate events, key rotation, and recovery;
- architecture dependency checks required in CI.

## Cross-phase safeguards

The following gates apply to every phase:

- ledger entries remain append-only;
- each booking remains balanced and idempotent;
- no module or service writes another owner's data directly;
- no remote provider participates in a money transaction;
- database changes are additive and backward compatible during rollout;
- tests prove both the new path and rollback path before traffic switches;
- secrets remain outside Git;
- the project remains runnable by one developer with Docker Compose.

## ADR sequence

| ADR | Expected phase | Decision |
| --- | --- | --- |
| ADR-001 | 0 | Evolutionary migration instead of big-bang rewrite |
| ADR-002 | 1 | Domain ownership and dependency boundaries |
| ADR-003 | 2 or 3 | Application ports and Banking Core unit of work |
| ADR-004 | 3 | Notification extraction and data contract |
| ADR-005 | 4 | Outbox, broker choice, delivery semantics, and deduplication |
| ADR-006 | 5 | Identity data split, token authority, and migration |
| ADR-007 | 6 | Gateway and service authentication |
| ADR-008 | 7 | Telemetry standards and sensitive-data policy |

Create an ADR only when the decision is actually made; do not pre-decide technology to fill the sequence.
