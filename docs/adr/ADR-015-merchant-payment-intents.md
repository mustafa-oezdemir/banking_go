# ADR-015: Immutable Merchant Payment Intents

## Status

Accepted.

## Context

The Banking UI must support a checkout redirect from a trusted merchant
backend. Query parameters are not a valid payment instruction boundary because
they can be altered in the browser.

## Decision

Banking owns the merchants and merchant_payment_intents tables. A merchant backend
uses POST /merchant/payment-intents with the independent MERCHANT_API_TOKEN;
the browser only receives the opaque payment_intent_id in approval_url.

The persisted intent holds the merchant, beneficiary account/IBAN, amount,
currency, merchant reference, expiry and eventual Banking payment ID. These
values are immutable. The authenticated customer can only select an owned
source account and explicitly confirm the demo payment at:

~~~text
GET  /merchant/payment-intents/{intentID}
POST /merchant/payment-intents/{intentID}/approve
~~~

Approval delegates to the existing Payment service. That service remains the
single owner of source-account authorization, VoP, idempotency, financial
state transitions, the local ACID booking transaction, ledger postings, and
the transactional outbox. No second money-movement implementation is created.

Merchant requests are idempotent by (merchant_id, merchant_reference).
Conflicting amount or currency retries are rejected. Intents expire after 30
minutes and have only one linked payment order.

## Consequences

- Browser-side amount, recipient, and reference tampering is rejected by the
  strict API contract and cannot affect the persisted intent.
- An opaque ID is not authorization for an existing customer payment; once an
  intent is linked, access is checked through the Payment owner boundary.
- The local pehlione-ecommerce merchant uses an internal demonstration
  settlement account. It is not an Identity login account.
- This change does not mark an e-commerce order as paid. A subsequent
  integration must consume a signed, idempotent Banking completion webhook
  (or a dedicated outbox consumer); browser return URLs are only UX.
