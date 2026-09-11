# ADR-009: Service Authentication Model

- Status: Accepted
- Date: 2026-09-11

## Decision

The browser continues to use an `HttpOnly`, `Secure` (outside local HTTP), `SameSite=Strict` cookie named `jwt`. Its value is a short-lived 15-minute HS256 JWT issued only by Identity. It includes `user_id`, `jti`, `iat`, `nbf`, `exp`, `iss=pehlione-identity`, and `aud=pehlione-banking-api`.

Banking validates signature, expiry, issuer and audience, then authorizes the resulting subject against its own Customer/account records. It never calls Identity during normal account access. The short lifetime limits logout and password-reset exposure; cookie clearing provides immediate browser logout. Refresh tokens are intentionally not introduced in this learning phase.

`JWT_SECRET` is shared only by Identity and Banking in local Compose. It must be 32+ random characters and is rotated by deploying Identity and Banking together with the new value; all outstanding access tokens then expire immediately. A production next step is asymmetric signing (Identity private key, Banking public key) with `kid`-based overlap; this avoids giving Banking signing capability.

The Identity-to-Banking provisioning call uses a separate 32+ character `INTERNAL_SERVICE_TOKEN`, compared in constant time. It is never browser-visible and must be rotated independently.
