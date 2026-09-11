# ADR-012: Enforce service data ownership with PostgreSQL roles

Status: Accepted

Context: Identity, Banking and Notification are independent processes, but their local development stack uses one PostgreSQL instance. Source-level discipline alone cannot prevent an accidental cross-schema query when every service connects as the administrator.

Problem: Preserve the Banking ACID unit of work while making private Identity and Notification persistence inaccessible to other runtime services.

Decision Drivers:

- prevent cross-service reads, joins and writes at the database boundary;
- keep local setup manageable for a single developer;
- avoid credentials in Git and retain additive, testable migrations;
- preserve Banking's Account/Payment/Ledger transaction boundary.

Options Considered:

1. Keep one shared administrator connection for every service.
2. Create a separate PostgreSQL instance/database for every service immediately.
3. Retain one local instance but use schema-local runtime roles, an administrator-only role-provisioner, and migrations.

Selected Decision: Option 3. `banking_app` owns `public`, `identity_app` owns `identity`, and `notification_app` owns `notification`. A short-lived Compose control-plane service creates/rotates only the runtime role passwords. Migration 000015 revokes public schema access and grants each role DML privileges only in its owned schema. Runtime services cannot use the administrator credentials.

Positive Consequences:

- unauthorized cross-service SQL fails even if code is faulty;
- credentials rotate independently and do not appear in repository files;
- existing local Banking transactions remain local and serializable;
- future split to separate databases requires no application query rewrite.

Negative Consequences:

- Compose has two additional startup control-plane services;
- migration ordering and role grants require maintenance;
- a single PostgreSQL administrator still exists for local infrastructure.

Risks:

- a new table may lack a grant if it bypasses the migration owner;
- an administrator credential compromise still reaches all schemas;
- shared-instance operational failures can affect all services.

Mitigations:

- default privileges grant new migration-owned tables only to their schema owner;
- integration and architecture fitness tests inspect the effective grants and wiring;
- administrator credentials are limited to provisioning/migrations and kept outside Git;
- separate physical databases remain the revisit path for stronger isolation.

Revisit Conditions: Move to independently provisioned databases when services require independent backup/restore, scaling, regulatory isolation, or a deliberately distributed-financial-transaction learning exercise.
