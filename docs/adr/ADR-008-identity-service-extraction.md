# ADR-008: Extract the Identity Service

- Status: Accepted
- Date: 2026-09-11

## Decision

`identity-service` owns registration, credential hashes, login, password-reset tokens and browser session issuance in the PostgreSQL `identity` schema. It is an independent container and has no access to Banking tables through its repository.

An **Identity User** is an authentication subject. A **Banking Customer** is a Banking-owned record that owns accounts and ledger authorization. They deliberately share a UUID at provisioning time, but neither model is imported as the other domain object.

After creating an Identity User, Identity invokes Banking's private, token-protected `POST /internal/customers/provision` command. Banking atomically creates the Customer, initial account and opening ledger entries. If that command fails, Identity deletes the just-created identity as a compensation. This is not a distributed transaction; registration is the only cross-service provisioning workflow and is retried safely by the stable subject UUID.

## Consequences

- Banking never connects to or queries `identity.users` or `identity.password_reset_tokens`.
- Existing demo records are copied once into the Identity schema by migration 000014 for local continuity; runtime credential reads are Identity-owned.
- Identity communicates with Notification only through its explicit command API.
- Identity availability is required for new login/registration, not for already issued short-lived Banking requests.
