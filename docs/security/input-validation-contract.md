# Input surface inventory and validation contract

Status: Phase 01 contract; implementation starts in later phases

Last reviewed: 2026-09-11

## Policy

Every value crossing a browser, HTTP, message, configuration or persistence boundary is untrusted. Client validation is for usability; the owning backend endpoint is the authoritative enforcement point. Values that represent human-readable names, addresses, descriptions, labels or references are plain text: HTML/markup, control characters and unsupported semantic formats are rejected rather than sanitized. Passwords, tokens and opaque identifiers are not subjected to a generic text allowlist.

Every JSON command follows: request-size limit → JSON media-type check → strict single-object decode → field normalization → field syntax/length validation → domain/business validation → authorization → mutation. SQL parameterization remains independent from this contract.

## Shared contract rules

| Rule | Contract |
| --- | --- |
| JSON | `application/json`, one document, unknown fields rejected, wrong types rejected. |
| Size | Banking global limit: 1 MiB; Identity and Notification command limit: 64 KiB. Individual fields have smaller limits below. |
| Plain text | Trim outer Unicode whitespace; reject empty/whitespace-only required values, NUL, CR/LF and HTML markup. Normalize only when semantics require it; do not silently change free text. |
| Identifiers | UUIDs use canonical parser; path/query IDs are never authorization. Opaque reset/JWT/service tokens have their own exact grammar. |
| Enums | Case-normalize only where existing domain behavior permits it, then reject outside the listed vocabulary. |
| Money | JSON decimal string only, EUR, positive, bounded and at most two input decimal places; never float. |
| Dates | RFC3339 for API execution timestamps; `YYYY-MM-DD` for standing-order dates; domain rules decide past/future/ordering. |
| HTML | Default `NO`. There is no rich-text field in this application. Output remains context-escaped by React/JSON encoding. |

## Browser forms and corresponding backend commands

| Field | Context / API | Type and rule | Required / bounds | Normalization | HTML | Backend owner |
| --- | --- | --- | --- | --- | --- | --- |
| `email` | register, login, forgot password | email address | required; bounded email length | trim, lower-case via identity normalization | NO | Identity |
| `full_name` | registration, profile | person plain text | registration optional fallback; profile 2–100 | trim and profile normalization | NO | Identity / Account |
| `password`, `new_password`, confirmation | register, login, reset | opaque password | register/reset 15–72 bytes; confirmation client-only equality | no character blacklist or trim | N/A | Identity |
| `token` | reset password | base64url reset token | required; decodes to exactly 32 bytes | trim outer whitespace | N/A | Identity |
| `name` | create/update account | account plain text | required; 1–100 | trim | NO | Banking Account |
| `full_name`, `phone`, `birth_date` | profile | person, phone, ISO date | profile-specific required/optional bounds | field-specific profile normalization | NO | Banking Account |
| `address_line1`, `address_line2`, `postal_code`, `city`, `country_code` | profile | postal address / ISO country code | 3–120, 0–120, 3–12, 2–80, exactly 2 respectively | trim; uppercase country | NO | Banking Account |
| `source_account_id`, account path IDs | payment, transfer, standing order | UUID | required where command selects account | trim then UUID parse | N/A | Banking Payment/Account |
| `beneficiary_name` / payee `name` | verify payee, payment, beneficiary, standing order | human plain text | required; 1–140 | trim | NO | Banking Payment |
| `beneficiary_iban` / `iban` | payment, payee, beneficiary, standing order | IBAN | required; country-aware IBAN grammar | remove permitted display spaces, uppercase | N/A | Banking Account/Payment |
| `beneficiary_bic` / `bic` | payment, beneficiary | BIC | optional unless destination rule requires it | uppercase, trim | N/A | Banking Payment |
| `purpose`, `creditor_reference`, beneficiary `category` | payment, standing order, beneficiary | reference/plain text | purpose 0–140; other field-specific bounds | trim | NO | Banking Payment |
| `amount` | transfer, payment, standing order, admin balance | decimal string | required, positive EUR, max business amount, 2 decimal places | decimal parser canonicalizes | N/A | Banking Ledger/Payment |
| `transfer_type` | payment, standing order | enum | `STANDARD` or `INSTANT` | uppercase, trim | N/A | Banking Payment |
| `schedule_type` | payment | enum | immediate/scheduled domain values | uppercase, trim | N/A | Banking Payment |
| `requested_execution_at` | payment | RFC3339 timestamp | required for scheduled payment | UTC parse | N/A | Banking Payment |
| `frequency`, `start_date`, `end_date`, `max_occurrences`, `status` | standing order | recurrence enum/date/integer/status | domain-required by end mode | trim/enumeration/date parse | N/A | Banking Payment |
| `accept_vop_mismatch`, `confirm_demo` | payment confirmation | boolean | required only by confirmation policy | none | N/A | Banking Payment |
| `role` | admin user mutation | enum | `CUSTOMER` or `ADMIN` | uppercase, trim | N/A | Banking admin |
| account `status` | admin account mutation | enum | `ACTIVE` or `BLOCKED` | uppercase, trim | N/A | Banking admin |
| admin `operation` | balance adjustment | enum | supported adjustment operation only | uppercase, trim | N/A | Banking Ledger/Admin |
| `Idempotency-Key` | payment create header | opaque bounded key | required, non-empty, bounded printable token | trim; no HTML validator | N/A | Banking Payment |
| `limit`, `offset`, SSE `Last-Event-ID` | list/query/SSE headers | integer or opaque event ID | explicit maximum and non-negative pagination | parse, bound | N/A | Banking HTTP |

## Internal command and event inputs

| Field / command | Boundary | Contract |
| --- | --- | --- |
| `X-Internal-Service-Token` + customer provision body | Identity → Banking | bearer token must be 32+ chars and constant-time compared; `identity_id` UUID, normalized email, plain-text full name; strict JSON and 64 KiB maximum. |
| Notification HTTP bearer and password-reset/activity commands | Banking/Identity → Notification | 32+ char service token, exact JSON media type, strict JSON, 64 KiB maximum; email and command-specific recipient/template validation. |
| RabbitMQ `EventEnvelope` | Banking outbox → Notification | immutable UUID event ID/aggregate ID, known type/version, RFC3339 occurrence, bounded correlation ID and strict payload contract. Invalid events dead-letter without provider delivery. |
| Environment configuration | operator → service | URL/port/duration/secret parsers fail fast. Never log raw configuration secrets. |

## Backend source map and current state

| Surface | Source | Existing baseline control | Contract follow-up |
| --- | --- | --- | --- |
| Banking JSON | `backend/internal/platform/httpapi/security.go`, handler files | strict decode, size limit, UUID/date/money/domain checks in multiple locations | Phase 03 centralizes plain-text/control/markup rules where useful and adds direct HTTP regression payloads. |
| Identity JSON | `backend/internal/platform/identityapi/handler.go` | 64 KiB strict decoder; identity email/password policy | Phase 03 adds field contract parity and media-type decision. |
| Notification JSON | `backend/internal/platform/notificationapi/handler.go` | strict media type, strict decode, 64 KiB limit, command validation | Phase 03/16 add regression coverage for recipient/header/template input. |
| Frontend forms | `frontend/app/auth/*`, `components/banking/BankingApp.tsx`, `TransferWizard.tsx` | browser `required`, length and type hints on some fields | Phase 02 aligns client validation/normalization with this contract without treating it as a security boundary. |

## Regression payload suite

Each Phase 02–04/23 test must choose an expectation from the field contract, not blindly ban strings. The common suite includes:

- plain-text markup candidates: `<script>alert(1)</script>`, `<img src=x onerror=alert(1)>`, `<svg/onload=alert(1)>`, and `"><script>alert(1)</script>`;
- SQL-like strings (`' OR 1=1--`, `"; DROP TABLE users; --`) against parameterized command paths;
- CR/LF, NUL, tab/newline and malformed Unicode/very long combining sequences where a field forbids them;
- empty, whitespace-only, maximum, maximum-plus-one and oversized request bodies;
- `-1`, `0`, extra decimal precision and extreme money values;
- unknown JSON properties, multiple JSON documents, wrong types and malformed UUID/date/enum values.

## Acceptance criteria for implementation phases

1. A direct HTTP request that bypasses Next.js receives a stable client error for a contract violation.
2. A valid current workflow retains its existing API shape and banking semantics.
3. No generic `<` blacklist is applied to passwords, opaque tokens, identifiers or values where it breaks valid semantics.
4. Every frontend validation introduced in Phase 02 has an authoritative backend equivalent by the end of Phase 03.
5. Tests prove rejected input cannot reach persistence, email delivery or event publication.
