# Security Policy

## Supported Version

Security fixes are applied to the latest commit on `main`.

## Reporting a Vulnerability

Do not open a public issue containing exploit details, credentials, or personal
data. Use GitHub's private vulnerability reporting for
[`mustafa-oezdemir/banking_go`](https://github.com/mustafa-oezdemir/banking_go/security/advisories/new).
Include the affected endpoint, reproduction steps, impact, and a minimal proof
of concept. Test only systems you own or are explicitly authorized to assess.

## Security Model

- Authentication uses signed, two-hour JWT sessions in `HttpOnly`,
  `SameSite=Strict` cookies. Production cookies are also `Secure`.
- Unsafe cookie-authenticated requests require `X-CSRF-Protection: 1` and pass
  Fetch Metadata and Origin checks.
- Every account read and mutation verifies ownership. System accounts cannot be
  accessed through user account endpoints.
- SQLC generates parameterized PostgreSQL queries; user input is never
  concatenated into SQL.
- Account and credential input is normalized and bounded. Passwords require at
  least 15 characters and bcrypt input is limited to 72 bytes.
- API and frontend responses set CSP, clickjacking, MIME-sniffing, referrer,
  permissions, and HTTPS transport headers.
- Requests are rate-limited by client IP, JSON-only where applicable, and
  limited to 1 MiB.
- The repository has no file-upload endpoint. Adding one requires independent
  content verification, size limits, generated filenames, isolated storage,
  malware scanning, and authorization tests.

## Automated Checks

GitHub Actions runs Gitleaks, `govulncheck`, and a production dependency audit.
Dependabot monitors Go, frontend, Docker, and GitHub Actions dependencies.

For a running local stack, execute:

```powershell
.\security-smoke.ps1
```

The suite checks security headers, unauthenticated access, media-type and body
limits, weak passwords, a SQL injection payload, cookie attributes, stored XSS
handling, CSRF defenses, cross-user IDOR protection, and cleanup. Remote targets
are refused unless the caller adds `-AllowRemote`; that switch is not a grant of
authorization.
