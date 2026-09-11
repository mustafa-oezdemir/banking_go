# Phase 05 — Authentication and Session Security

## Result

Identity is the only runtime component that accepts credentials, hashes or verifies passwords, issues JWTs, sets the session cookie, and handles logout or password reset. Gateway routes every credential endpoint to Identity. Banking only verifies the signed token and applies current customer, ownership, account-status, and role authorization.

## Flow inventory

| Flow | Owner | Security properties |
| --- | --- | --- |
| Register | Identity | Strict JSON, normalized email/name, 15–72 byte password policy, bcrypt, bounded IP rate limit, customer provisioning compensation, fresh JWT/JTI. |
| Login | Identity | Generic invalid-credential response, dummy bcrypt work for unknown identities, bounded IP rate limit, fresh JWT/JTI. |
| Logout | Identity | Valid and current JWT required; Identity increments the authoritative generation, Banking is set to that exact value, and the cookie is expired. Replaying the old JWT is rejected. |
| Password change | Identity | Valid current session, CSRF protection, current password verification, different strong new password, bcrypt replacement, exact-version synchronization to Banking, cookie expiry, explicit sign-in required. |
| Password reset | Identity | Generic asynchronous request response, hashed one-time token, 15-minute expiry, Identity session generation increment, exact-version synchronization to Banking, cookie expiry. |
| Session/authorization | Banking | HS256 signature, fixed issuer/audience/algorithm and expiry validation, customer existence and synchronized session-generation check; administrator role is re-read for every admin request. |

## JWT and cookie contract

- Algorithm: fixed `HS256`; keys shorter than 32 characters are rejected at startup.
- Claims: `iss=pehlione-identity`, `aud=pehlione-banking-api`, random `jti`, `iat`, `nbf`, `exp`, `user_id`, and `session_version`.
- Lifetime: 15 minutes; no refresh token exists.
- Cookie: host-only `jwt`, `Path=/`, `HttpOnly`, `SameSite=Strict`, and `Secure` behind HTTPS; auth responses use `Cache-Control: no-store`.
- Mutation protection: cookie-authenticated logout and password change require `X-CSRF-Protection: 1` and reject cross-site Fetch Metadata. Bearer-token clients are not subject to the browser CSRF rule.

## Extraction and duplicate implementation review

The former Banking handlers for register, login, logout, forgot-password, and reset-password were removed together with Banking-side JWT issuance and its legacy Identity repository adapter. `backend/cmd/main.go` registers none of these routes. This matters because `docker-compose.dev.yml` can expose a Banking development process directly; Gateway routing alone was not a sufficient boundary. A static architecture test fails if any credential route is registered again in the Banking executable.

The Banking `public.users.hashed_password` column and old reset-token migration/table remain compatibility data pending a dedicated destructive data migration. No Banking runtime handler reads those credentials. Removing the schema artifacts requires a separately reviewed migration because bootstrap and rollback compatibility still reference them.

Banking retains a synchronized `session_version` projection so it can reject revoked tokens without a synchronous Identity dependency on every financial request. Logout, password change, password reset, and login use an authenticated private command that sets Banking to Identity's exact version, rather than incrementing it. Repeated commands therefore cannot drift the two projections. This is not a distributed transaction: an infrastructure failure can temporarily fail closed; a later login repeats the exact-version synchronization before issuing a cookie.

Role changes do not mutate the Banking session generation because Identity is the generation owner. Administrator authorization reads the current role from PostgreSQL on every request, so demotion takes effect immediately without creating Identity/Banking version drift. Blocked account status is likewise evaluated from the current account row by payment and account operations.

## Automated evidence

- Identity tests assert issuer, audience, expiry, random JTI, fresh-token issuance, cookie flags, `no-store`, CSRF enforcement, current-password verification, logout revocation, and replay rejection.
- JWT contract tests reject wrong issuer, wrong audience, wrong signing algorithm, and expired tokens.
- Gateway tests route all credential endpoints, including `/change-password`, exclusively to Identity.
- Banking architecture tests reject re-registration of legacy credential routes.

## Remaining production-bank controls

- Replace the shared HS256 secret with asymmetric signing in KMS/HSM, `kid`, JWKS distribution, and controlled rotation.
- Add refresh-token rotation, device/session inventory, selective session revocation, MFA/WebAuthn, credential-stuffing detection, breached-password screening, and risk-based step-up authentication.
- Replace in-memory per-instance rate limiting with a trusted-edge/distributed limiter and a formally configured proxy trust chain.
