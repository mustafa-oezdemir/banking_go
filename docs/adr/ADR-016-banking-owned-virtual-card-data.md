# ADR-016: Banking-Owned Virtual Card Data

- Status: Accepted
- Date: 2026-09-14
- Scope: Banking virtual-card management and merchant checkout

## Context

Customers need multiple virtual cards linked to their EUR accounts, the ability to reveal card details in the authenticated Banking UI, and lifecycle controls to cancel and delete cards. E-Commerce must not persist PAN or CVC data. A CVC must still be supplied for every merchant authorization.

## Decision

The Banking service is the only owner of card credentials. PAN and CVC are encrypted with AES-256-GCM before PostgreSQL persistence. The encryption and fingerprint key comes from `CARD_DATA_KEY`; plaintext credentials are never written to application logs.

The ordinary card-list endpoint returns only metadata and the last four digits. `GET /cards/{id}/credentials` performs an owner-scoped lookup, is rate limited, and sends `Cache-Control: no-store`. The frontend holds revealed credentials only in component memory and hides them after 60 seconds.

Customers may issue multiple active cards for the same eligible EUR account. Cancellation is permanent and invalidates merchant authorization. Deletion is allowed only after cancellation and cascades deletion of the Banking-owned merchant tokens for that card.

E-Commerce receives and stores only an opaque, merchant-bound token and display metadata. Tokenization and every payment authorization send the CVC to Banking for validation. Banking decrypts the stored CVC only for the comparison and uses a constant-time comparison. It does not return the CVC to E-Commerce.

## Consequences

- Losing or changing `CARD_DATA_KEY` makes existing encrypted card credentials unusable; production deployment therefore requires managed key storage, backup, access control, and a planned key-rotation mechanism.
- Cards created before this migration have no recoverable encrypted credentials and must be reissued if reveal or merchant authorization is required.
- This design limits accidental disclosure in the demo architecture, but does not by itself make the project PCI DSS compliant. A production card issuer would require HSM/KMS-backed keys, strict PCI scope controls, auditing, monitoring, retention rules, and an independent assessment.
