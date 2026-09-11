# Evolutionary Migration Roadmap

Status: Architecture Phase 10 completed

Last updated: 2026-09-11

## Working agreement

Only one phase is active at a time. Every phase ends with documentation updates, relevant tests and static/security checks, affected builds, Docker validation, diff review, a meaningful commit, and a successful push. A failed push stops the roadmap.

Backward compatibility is preferred. Financial behavior is changed only with explicit tests and an ADR. Account, Payment, and Ledger remain one deployable unit until a documented requirement outweighs the loss of local ACID transactions.

## Phase overview

| Phase | Outcome | Production topology change |
| --- | --- | --- |
| 0. Baseline analysis | Completed: current/target architecture, risks, invariants, and migration decision documented | None |
| 1. Establish domain boundaries | Completed: explicit Identity, Customer/Account, Payment, Ledger, Notification, and Platform modules with an import fitness test | None |
| 2. Modularize banking backend | Completed: critical domain rules are pure; Ledger, Profile, and Authentication use application ports; payment ACID exception is documented | None |
| 3. Extract notification service | Completed: independent HTTP service owns SMTP/Resend delivery and reads no Banking tables | One new service |
| 4. Introduce async notification events | Completed: durable outbox, broker, inbox/deduplication, retries | RabbitMQ |
| 5. Extract identity service | Completed: credentials and sessions are owned by an Identity contract and schema | One new service |
| 6. Introduce gateway and service auth | Completed: stable external routing and authenticated internal calls | Gateway |
| 7. Add distributed observability | Completed: cross-service logs, metrics and traces | Jaeger |
| 8. Data ownership hardening | Completed: schema-local PostgreSQL runtime roles and ownership fitness tests | Two short-lived control-plane containers |
| 9. Contract & failure testing | Completed: distributed fitness functions and failure-path tests | None required |
| 10. Final architecture review | Completed: retain Banking Core decision matrix and final C4 views | No new Banking service |

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

Completion: implemented on 2026-09-10. ADR-003 records the deliberately incremental boundary and the payment unit-of-work exception.

Delivered:

- pure posting, lifecycle, payment-intent, ownership, and profile rules with infrastructure-free tests;
- a module-owned Ledger repository port with PostgreSQL transaction implementation in `platform/database`;
- Profile and Authentication application services with narrow repository ports;
- sqlc-to-application mappings at the PostgreSQL boundary;
- injected clocks for payment validation and profile updates where determinism matters;
- architecture tests that prohibit infrastructure dependencies in core packages;
- a documented payment unit-of-work exception that preserves serializable booking atomicity.

Exit criteria:

- domain packages do not import HTTP, Chi, sqlc, PostgreSQL, SMTP/Resend, or process environment packages;
- HTTP and PostgreSQL are adapters at the extracted use-case boundaries;
- no topology or data ownership change yet.

## Phase 3 — Extract notification service (completed)

Goal: isolate a failure-tolerant operational capability without risking financial consistency.

Delivered:

- independent executable/container, health endpoint, configuration, timeouts, structured logs, request IDs, and graceful shutdown;
- token-authenticated, versioned password-reset and account-activity HTTP commands;
- explicit recipient/template payloads resolved inside Banking so Notification never queries Banking private tables;
- SMTP/MailHog and Resend provider ownership moved to Notification;
- post-commit failure handling and contract/no-retry tests;
- no broker and no unsafe retry in this phase.

Rollback: disable `NOTIFICATION_SERVICE_URL` to use the no-op sender, or restore the prior in-process adapter while keeping financial commit behavior unchanged.

## Phase 4 — Introduce asynchronous notification events (completed)

Goal: guarantee that committed banking facts can be delivered at least once without coupling transactions to provider availability.

Delivered:

- transactional outbox rows in the same serializable transaction as payment, ledger, and balance writes;
- RabbitMQ durable topic exchange with persistent publisher-confirmed event publication;
- `payment.booked.v1` and `payment.failed.v1` envelopes, each with an active Notification email consumer;
- Notification-owned processed-event ID store with leased claims for idempotent consumption;
- bounded delayed retries and a durable poison-message dead-letter queue;
- contract tests plus Compose duplicate-delivery and broker-down recovery verification.

Exit criteria:

- committed financial transactions insert their notification event before commit;
- normal duplicate broker delivery does not create a second provider call;
- broker outage delays delivery without rolling back or blocking ledger booking.

## Phase 5 — Extract identity service (completed)

Goal: move credentials, login/session policy, and password reset behind an independently owned service and database.

Delivered:

- independent `identity-service` executable/container with its own `identity` schema;
- registration, bcrypt credential handling, login, password-reset tokens and cookie/JWT issuance moved behind the Identity HTTP contract;
- explicit Identity User versus Banking Customer boundary, with one private idempotent provisioning command;
- existing local credential records copied once during additive migration 000014;
- contract tests for registration, login and invalid credentials, plus gateway-level unauthorized-access verification.

Identity is required for authentication changes, but Banking validates an already issued 15-minute access token locally and never reads the Identity schema.

## Phase 6 — Introduce gateway and service authentication (completed)

Goal: keep browser contracts stable as internal deployables appear.

Delivered:

- small Go gateway is the only browser-facing backend port; Identity and Banking ports are private to Compose;
- identity paths route to Identity while all Banking routes preserve their existing shape;
- request IDs, body limits, browser headers, bounded upstream handling and Prometheus-compatible gateway metrics are enforced at the edge;
- the private Identity-to-Banking provisioning call uses a separate constant-time-compared service token.

## Phase 7 — Distributed observability (completed)

Goal: make a multi-process request and asynchronous event traceable end to end.

Delivered:

- structured service logs and propagated request/correlation IDs;
- OpenTelemetry W3C trace propagation from Gateway through Identity and Banking to Jaeger via OTLP/HTTP;
- gateway request/failure counters and latency logs, plus a Banking outbox-backlog gauge;
- independent health checks and operational runbooks for PostgreSQL, RabbitMQ, Notification and outbox failure scenarios.

PII, secrets, JWTs, raw tokens, and unmasked financial identifiers remain excluded from telemetry.

## Phase 8 — Data ownership hardening (completed)

Delivered:

- Banking, Identity and Notification run under separate PostgreSQL roles with DML rights only for `public`, `identity`, and `notification` respectively;
- an administrator-only role provisioner and migrator run before application services; their passwords remain outside Git;
- migration 000015 revokes public schema access, grants schema-local rights, and supplies default privileges for future migrations;
- automated architecture and integration checks verify Compose role wiring and effective cross-schema denial;
- [service-data-ownership.md](service-data-ownership.md) and ADR-012 document the boundaries.

## Phase 9 — Contract & failure testing (completed)

Delivered:

- a versioned set of distributed fitness functions mapping notification, RabbitMQ, outbox, Identity, PostgreSQL, idempotency and malformed-event scenarios to executable tests and manual recovery checks;
- focused failure-path tests proving a malformed event cannot reach the provider and an Identity outage does not block a Banking route;
- an explicit balance invariant for every booking, plus existing concurrent scheduled-payment, idempotency and duplicate-consumer tests.

See [distributed-fitness-functions.md](distributed-fitness-functions.md).

## Phase 10 — Final architecture review (completed)

Delivered:

- ADR-014, which records the decision to retain Account, Payment and Ledger in one Banking Core;
- a decision matrix with extraction and revisit criteria for Identity, Notification, Account, Payment and Ledger;
- final C4-style context, container/data ownership, and Banking Core component diagrams.

See [final-service-extraction-decision.md](final-service-extraction-decision.md).

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
| ADR-003 | 2 | Domain/application separation, ports, and the retained payment unit of work |
| ADR-004 | 3 | Notification extraction, private HTTP contract, and failure semantics |
| ADR-005 | 4 | RabbitMQ messaging topology and publisher confirmation |
| ADR-006 | 4 | Transactional outbox and database/broker consistency boundary |
| ADR-007 | 4 | At-least-once idempotent Notification consumer semantics |
| ADR-008 | 5 | Identity service extraction and data ownership |
| ADR-009 | 5 | Browser and service authentication model |
| ADR-010 | 6 | API gateway scope and routing |
| ADR-011 | 7 | Observability stack and sensitive-data policy |
| ADR-012 | 8 | Enforced service data ownership with PostgreSQL roles |
| ADR-013 | Observability follow-up | RabbitMQ monitoring with Prometheus and Grafana |
| ADR-014 | 10 | Retain Account, Payment and Ledger as one Banking Core |

Create an ADR only when the decision is actually made; do not pre-decide technology to fill the sequence.
