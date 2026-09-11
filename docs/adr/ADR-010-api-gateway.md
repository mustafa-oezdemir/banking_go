# ADR-010: API Gateway

- Status: Accepted
- Date: 2026-09-11

## Decision

After the second public backend service, a small Go `gateway` provides the single browser-facing API entry point. It routes only identity paths (`/register`, `/login`, `/logout`, `/forgot-password`, `/reset-password`) to Identity and all other paths to Banking. Notification remains private.

The gateway adds/carries `X-Request-ID`, limits request bodies to 1 MiB, supplies basic browser security headers, uses bounded upstream failures, and exposes `/metrics`. It has no banking business rules, persistence, authorization decisions, or token minting.
