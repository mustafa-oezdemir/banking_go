
Evet. Bu proje için en sağlıklı yöntem **her güvenlik aşamasını bağımsız bir değişiklik paketi olarak tamamlayıp test ettikten sonra ayrı commit + push etmek**. Böylece bir aşama sorun çıkarırsa geri almak, diff'i incelemek ve hangi güvenlik kontrolünün ne zaman eklendiğini görmek kolay olur. Proje talimatındaki yaklaşım da güvenliği mimari, uygulama, API, veri, altyapı, DevSecOps ve detection katmanları halinde ele almayı istiyor.  API kaynağı da AuthN/AuthZ, validation, error handling, rate limiting, data exposure, business logic ve secure coding'i ayrı test alanları olarak ele alıyor.

Ben bunu **18 aşamalık bir security-hardening roadmap** olarak kurardım.

## Git çalışma modeli

Doğrudan `main` üzerine geliştirme yapma. Her aşama için:

```text
main
  ↓
security/phase-01-baseline
  ↓ PR/Merge
main
  ↓
security/phase-02-docker-hardening
  ↓ PR/Merge
main
  ↓
security/phase-03-secrets
...
```

Her aşamada standart akış:

```bash
git checkout main
git pull

git checkout -b security/phase-XX-name

# değişiklikler

# testler
go test ./...
# frontend testleri varsa
# lint/security scan

git status
git diff

git add .
git commit -m "security(phase-XX): description"

git push -u origin security/phase-XX-name
```

Aşama **testler PASS olmadan push edilmemeli**. Push'tan sonra PR/merge yapılır; sonraki aşama güncel `main` üzerinden başlar.

---

# PHASE 00 — Security Baseline & Architecture Inventory

**Amaç:** Hiçbir kodu değiştirmeden mevcut security baseline'ı dondurmak.

İncelenecekler:

```text
backend/
frontend/
docker-compose*
Dockerfile*
.env.example
migrations/
GitHub Actions
JWT
CSRF
AuthN/AuthZ
payments
ledger
email
password reset
rate limiter
logging
tests
```

Çıktılar:

```text
docs/security/
├── architecture.md
├── attack-surface.md
├── trust-boundaries.md
├── security-invariants.md
└── security-roadmap.md
```

Özellikle şu invariantlar yazılı hale getirilmeli:

```text
Customer → yalnız kendi hesapları
Customer → system account erişimi yok
Admin → server-side authorization
Debit total == Credit total
Transaction atomic
Idempotency korunuyor
Financial values → decimal
JWT revocation korunuyor
CSRF korunuyor
Secrets repository'de yok
```

Test baseline:

```bash
go test ./...
```

Commit:

```text
security(phase-00): establish security baseline and architecture inventory
```

Branch:

```text
security/phase-00-baseline
```

Bu aşamada güvenlik açığı düzeltilmez.

---

# PHASE 01 — Docker Network & Service Exposure

**Amaç:** PostgreSQL/MailHog gibi internal servislerin yanlışlıkla dış dünyaya açılmasını engellemek.

Kontroller:

```text
PostgreSQL published port
MailHog published port
0.0.0.0 bindings
Docker internal networks
development vs production compose
default database credentials
```

Development gerekiyorsa:

```yaml
127.0.0.1:${DB_PORT}:5432
```

Production'da tercih:

```text
backend → internal Docker network → PostgreSQL

Internet ─X→ PostgreSQL
Internet ─X→ MailHog
```

Eklenecek:

```text
docker-compose.dev.yml
docker-compose.prod.yml
```

veya mevcut yapı güvenli şekilde ayrılacak.

Acceptance:

```text
[PASS] DB production host'a publish edilmiyor
[PASS] MailHog production'a dahil değil
[PASS] Dev DB sadece localhost'ta
[PASS] Backend DB'ye internal network ile ulaşıyor
[PASS] mevcut development workflow çalışıyor
```

Commit:

```text
security(phase-01): harden docker network and service exposure
```

Branch:

```text
security/phase-01-docker-hardening
```

---

# PHASE 02 — Secrets & Secure Configuration

Repository genelinde ara:

```text
password
secret
token
api_key
private_key
JWT
DATABASE_URL
SMTP
RESEND
credential
```

Amaç:

```text
source code
    X
production secrets

environment / secret manager
    ↓
application
```

Production için:

* insecure default secret olmamalı
* demo credential production'da kullanılamamalı
* JWT secret validation
* DB credential validation
* email API secrets
* environment separation
* `.gitignore`
* startup fail-closed

Örnek:

```go
if production && jwtSecret == "" {
    return errors.New("JWT secret required")
}
```

Ama gerçek secret commit edilmez.

Acceptance:

```text
[PASS] hardcoded production secret yok
[PASS] production insecure defaults kabul etmiyor
[PASS] .env git tarafından ignore ediliyor
[PASS] .env.example sadece fake değer içeriyor
[PASS] secret eksikse production startup fail ediyor
```

Commit:

```text
security(phase-02): enforce secure secrets and configuration defaults
```

---

# PHASE 03 — SMTP / Email Transport Security

SMTP katmanını düzenle.

İki açık mod olsun:

```text
Development:
localhost MailHog
plaintext açıkça izin verilmiş

Production:
SMTP/API provider
TLS zorunlu
certificate verification zorunlu
```

Örneğin config:

```text
SMTP_TLS_REQUIRED=true
SMTP_ALLOW_INSECURE_LOCAL=false
```

Kural:

```text
remote host + plaintext SMTP
        ↓
     REJECT
```

`InsecureSkipVerify: true` gibi kestirme kullanılmamalı.

Acceptance:

```text
[PASS] MailHog local çalışıyor
[PASS] remote plaintext SMTP reddediliyor
[PASS] TLS certificate doğrulanıyor
[PASS] password reset email çalışıyor
```

Commit:

```text
security(phase-03): enforce secure email transport
```

---

# PHASE 04 — Password Reset Security

Burada sadece response metnine değil bütün flow'a bakılır:

```text
request
 ↓
normalize email
 ↓
lookup
 ↓
token generation
 ↓
storage
 ↓
email
 ↓
generic response
```

Kontrol:

* account enumeration
* timing discrepancy
* reset token entropy
* token expiration
* token single use
* old token invalidation
* brute-force protection
* token logging
* password reset sonrası session revocation

Özellikle mevcut olmayan kullanıcı ile olan kullanıcı arasındaki timing farkı incelenir.

Tercihen:

```text
HTTP request
    ↓
generic response
    ↓
background email processing
```

Acceptance:

```text
[PASS] aynı HTTP status
[PASS] aynı generic message
[PASS] reset token loglanmıyor
[PASS] token tek kullanımlık
[PASS] expiration var
[PASS] reset sonrası eski sessions iptal
[PASS] account enumeration minimize edilmiş
```

Commit:

```text
security(phase-04): harden password reset workflow
```

---

# PHASE 05 — API Data Minimization

Özellikle transaction response.

Domain model doğrudan client'a verilmemeli.

Yanlış model:

```text
Database LedgerEntry
      ↓
JSON
```

Doğru:

```text
Database LedgerEntry
      ↓
business mapping
      ↓
CustomerTransactionResponse
```

Customer'a gereksiz:

```text
internal account UUID
settlement account ID
system account ID
internal bookkeeping metadata
other customer's internal identifier
```

dönmemeli.

Acceptance:

```text
[PASS] customer transaction bilgilerini görüyor
[PASS] internal settlement UUID görünmüyor
[PASS] system account ID görünmüyor
[PASS] authorization değişmedi
[PASS] API regression tests geçiyor
```

Commit:

```text
security(phase-05): minimize sensitive transaction response data
```

Bu alan API pentesting kaynaklarındaki sensitive-data exposure ve property/object authorization yaklaşımıyla doğrudan uyumlu.

---

# PHASE 06 — BOLA / BFLA / Authorization Regression

Şimdi tüm API endpointleri envantere alınacak.

Matrix:

| Endpoint                 | Customer |    Admin | Anonymous |
| ------------------------ | -------: | -------: | --------: |
| Own account              |       ✅ | politika |        ❌ |
| Other customer's account |       ❌ | politika |        ❌ |
| System account           |       ❌ | politika |        ❌ |
| Admin endpoint           |       ❌ |       ✅ |        ❌ |

Her object-based endpoint test edilmeli:

```text
/accounts/{id}
/transactions/{id}
/payments/{id}
...
```

Test paterni:

```text
User A creates/accesses Object A
User B tries Object A
→ 403/404
```

Admin endpointlerinde BFLA testleri yapılır.

Acceptance:

```text
[PASS] cross-user access yok
[PASS] system account erişimi yok
[PASS] customer admin fonksiyonuna erişemiyor
[PASS] role yalnız frontend tarafından belirlenmiyor
[PASS] UUID randomness security control olarak kullanılmıyor
```

Commit:

```text
security(phase-06): strengthen authorization regression coverage
```

---

# PHASE 07 — Strict JSON & Input Validation

Bütün request DTO'lar incelenir.

Kontrol:

```text
unknown fields
trailing JSON
empty values
oversized strings
invalid UUID
negative amount
zero amount
currency
IBAN
pagination
enum
duplicate/ambiguous fields
```

Merkezi parser düşün:

```go
func DecodeJSONStrict(...)
```

Mümkünse:

```go
decoder.DisallowUnknownFields()
```

ve tek JSON document zorunluluğu.

Validation katmanları ayrılmalı:

```text
Syntax validation
        ↓
Domain validation
        ↓
Authorization
        ↓
Business operation
```

Acceptance:

```text
[PASS] unknown fields reddediliyor
[PASS] malformed JSON reddediliyor
[PASS] trailing JSON reddediliyor
[PASS] amount bounds uygulanıyor
[PASS] pagination limits var
[PASS] business logic bozulmadı
```

Commit:

```text
security(phase-07): centralize strict request validation
```

---

# PHASE 08 — Secure Error Handling

Amaç:

Client:

```json
{
  "error": "invalid_request"
}
```

görmeli.

Şunları görmemeli:

```text
SQL query
stack trace
filesystem path
internal host
DB schema
Go panic
secret
```

Server tarafında `request_id` ile detaylı log olabilir.

Kontrol:

```text
400
401
403
404
409
422
429
500
```

tutarlı mı?

Acceptance:

```text
[PASS] stack trace client'a gitmiyor
[PASS] SQL error client'a gitmiyor
[PASS] request ID mevcut
[PASS] internal log debugging için yeterli
```

Commit:

```text
security(phase-08): standardize secure API error handling
```

---

# PHASE 09 — Rate Limiting & Anti-Automation

Önce endpoint sınıfları:

```text
/login
/password-reset
/password-reset/confirm
/payment
/payment-confirm
/admin
/public API
```

Tek IP key yeterli değil.

Model:

```text
IP
+
account/email
+
authenticated user
+
endpoint
```

Production horizontal scaling için:

```text
API instance 1 ─┐
API instance 2 ─┼→ Redis/shared limiter
API instance 3 ─┘
```

Ama Redis migration ayrı bir internal commit gerekiyorsa aynı phase içinde iki commit olabilir; **push yine phase sonunda** yapılmalı.

Acceptance:

```text
[PASS] login brute force sınırlandırılıyor
[PASS] reset abuse sınırlandırılıyor
[PASS] payment abuse sınırlandırılıyor
[PASS] client X-Forwarded-For spoof edemiyor
[PASS] trusted proxy tanımı explicit
[PASS] production distributed architecture destekleniyor
```

Commit:

```text
security(phase-09): harden rate limiting and abuse protection
```

---

# PHASE 10 — JWT, Cookie, Session & CSRF Hardening

Mevcut iyi kontroller korunacak.

Kontrol:

```text
JWT algorithm
issuer
audience
expiration
nbf
jti
session_version
cookie Secure
cookie HttpOnly
SameSite
CSRF
Origin
Fetch Metadata
CORS
logout revocation
role changes
password changes
```

Özellikle regression amaçlı.

Cookie production:

```text
Secure=true
HttpOnly=true
SameSite=Strict/Lax based on architecture
```

CORS wildcard + credentials kombinasyonu engellenmeli.

Acceptance:

```text
[PASS] expired JWT reddediliyor
[PASS] invalid signature reddediliyor
[PASS] wrong issuer/audience reddediliyor
[PASS] session revocation çalışıyor
[PASS] CSRF protection çalışıyor
[PASS] untrusted Origin reddediliyor
```

Commit:

```text
security(phase-10): harden session jwt csrf and cors controls
```

---

# PHASE 11 — Database Security

PostgreSQL katmanı.

Kontrol:

```text
runtime DB user
migration DB user
least privilege
TLS
network exposure
SQL parameterization
schema permissions
backup
restore
audit
connection limits
```

Önerilen ayrım:

```text
migration_user
  └── CREATE/ALTER

application_user
  └── SELECT/INSERT/UPDATE gerekli tablolar

admin
  └── uygulama tarafından kullanılmaz
```

Acceptance:

```text
[PASS] app superuser kullanmıyor
[PASS] DB public değil
[PASS] parameterized queries
[PASS] production TLS planı
[PASS] backup/restore dokümante
```

Commit:

```text
security(phase-11): enforce database least privilege and hardening
```

---

# PHASE 12 — Financial Business Logic & Race Conditions

Bu proje için kritik phase.

İnvariant:

```text
Σ debit == Σ credit
```

Testler:

```text
concurrent transfer
duplicate payment
same idempotency key
different payload + same idempotency key
insufficient balance
blocked account
system account
zero payment
negative payment
very large payment
simultaneous cancellation/confirmation
```

Özellikle:

```text
Transaction A
Transaction B
     ↓ simultaneously
same balance
```

senaryosu test edilir.

Korunacak:

```text
SERIALIZABLE
row locks
idempotency
double-entry
state machine
```

Acceptance:

```text
[PASS] double spending oluşmuyor
[PASS] ledger dengeli
[PASS] duplicate request duplicate transfer yaratmıyor
[PASS] blocked account ödeme yapamıyor
[PASS] state transition bypass edilemiyor
```

Commit:

```text
security(phase-12): strengthen payment and ledger integrity
```

Business-logic ve parameter-tampering testleri API pentesting kaynağında ayrı kritik alan olarak ele alınıyor.

---

# PHASE 13 — MFA / Step-Up Authentication

Burada hemen büyük kod refactor'u yapılmamalı.

Önce architecture:

```text
Login
 ↓
Password/passkey
 ↓
normal session

Sensitive payment/admin action
 ↓
step-up authentication
 ↓
short-lived authorization
```

Tercih:

```text
WebAuthn / Passkeys
```

SMS OTP tek güçlü faktör olarak tasarlanmamalı.

Kritik fonksiyonlar:

```text
payment confirmation
password change
email change
new beneficiary
admin operations
high-risk session
```

Acceptance:

```text
[PASS] MFA enrollment flow
[PASS] MFA recovery strategy
[PASS] step-up auth
[PASS] replay protection
[PASS] recovery MFA'yı bypass etmiyor
```

Commit:

```text
security(phase-13): add step-up authentication foundation
```

---

# PHASE 14 — Transaction Authorization / SCA

Login MFA ile payment authorization aynı şey değil.

Örneğin kullanıcı şunu onaylamalı:

```text
Alıcı: Alice GmbH
IBAN: DE...
Tutar: 850.00 EUR
```

Onay:

```text
amount
beneficiary
IBAN
currency
transaction ID
```

ile bağlanmalı.

Server daha sonra bu değerleri değiştirerek aynı authorization'ı kullanamamalı.

Threats:

```text
transaction tampering
replay
session theft
beneficiary replacement
amount replacement
```

Acceptance:

```text
[PASS] authorization payment'a bağlı
[PASS] amount değiştirilirse authorization invalid
[PASS] beneficiary değiştirilirse invalid
[PASS] replay engelleniyor
```

Commit:

```text
security(phase-14): bind transaction authorization to payment details
```

---

# PHASE 15 — Security Logging & Audit Trail

Blue-team kısmı burada başlıyor. Proje kaynakları güvenli geliştirmeyi detection ve response ile tamamlamayı şart koşuyor; Incident Response için Preparation → Identification → Containment → Eradication → Recovery → Lessons Learned modeli temel alınabilir.

Structured events:

```json
{
  "event_type": "auth.login.failed",
  "timestamp": "...",
  "request_id": "...",
  "actor_id": "...",
  "source_ip": "...",
  "result": "failure"
}
```

En az:

```text
auth.login.success
auth.login.failed
auth.password_reset.requested
auth.password_reset.completed
auth.session.revoked

authorization.denied

payment.created
payment.confirmed
payment.cancelled

admin.role_changed
admin.account_blocked
admin.account_unblocked

security.rate_limit_triggered
security.invalid_token
```

Loglanmayacak:

```text
password
JWT
cookie
reset token
API key
private key
full card data
```

Commit:

```text
security(phase-15): add structured security audit logging
```

---

# PHASE 16 — Detection & Incident Response

Logging'in üstüne detection kurulur.

Örnek:

```text
20 failed login / 5 min
        ↓
Credential stuffing alert

50 authorization denied / user
        ↓
BOLA probing alert

10 password reset request / account
        ↓
Account attack alert

Unusual admin action
        ↓
Privileged activity alert
```

Dokümanlar:

```text
docs/security/
├── incident-response.md
├── detection-catalog.md
└── security-events.md
```

Her detection:

```text
Attack
→ Telemetry
→ Rule
→ Alert
→ Investigation
→ Response
```

Acceptance:

```text
[PASS] detection scenarios dokümante
[PASS] logs detection için yeterli
[PASS] runbooks mevcut
[PASS] containment prosedürü tanımlı
```

Commit:

```text
security(phase-16): add detection and incident response capabilities
```

---

# PHASE 17 — DevSecOps / Supply Chain / Final Security Gate

Son aşama kod tabanını sürekli koruyan CI katmanı.

Backend:

```bash
go test ./...
go vet ./...
staticcheck ./...
govulncheck ./...
```

Security araçları:

```text
Gitleaks       → secrets
govulncheck    → Go vulnerabilities
Semgrep        → SAST
Trivy          → container/filesystem/IaC
npm audit      → frontend dependency visibility
```

Mümkünse:

```text
SBOM
dependency update automation
branch protection
required reviews
required CI
```

Pipeline örneği:

```text
Push / PR
   ↓
Unit tests
   ↓
Lint
   ↓
SAST
   ↓
SCA
   ↓
Secret scan
   ↓
Container scan
   ↓
Build
   ↓
Integration/security tests
```

Scanner çıktısı otomatik olarak confirmed vulnerability sayılmaz; proje metodolojisi de scanner bulgusu ile doğrulanmış finding'i ayırmayı gerektiriyor.

En sonunda full regression:

```text
Authentication
Authorization
BOLA
BFLA
JWT
Sessions
CSRF
CORS
Injection
Validation
Rate limiting
Password reset
Sensitive exposure
Business logic
Race conditions
Ledger
Docker
DB
Secrets
Logging
CI/CD
```

Son dokümanlar:

```text
SECURITY.md
CYBER_SECURITY.md
SECURITY_TEST_PLAN.md
THREAT_MODEL.md
PRODUCTION_SECURITY.md
```

Commit:

```text
security(phase-17): establish final devsecops and security regression gate
```

---

## Her phase için agent'a vereceğin ortak kural

Bunu master prompt'un başına koy:

```text
IMPORTANT GIT WORKFLOW

This project is improved incrementally.

Each PHASE is an independent security delivery.

For every phase:

1. Pull the latest main branch.
2. Create:
   security/phase-XX-short-name
3. Analyze the current implementation before modifying code.
4. Do not assume previously reported vulnerabilities still exist.
5. Make only changes required for the current phase.
6. Add/update relevant tests.
7. Run all affected existing tests.
8. Run security regression tests.
9. Do not continue if tests fail.
10. Review git diff for secrets, debug code and unintended changes.
11. Commit using:
    security(phase-XX): <summary>
12. Push the branch to origin.
13. Report:
    - branch
    - commit hash
    - files changed
    - tests executed
    - PASS/FAIL
    - security improvement
    - residual risk
14. STOP.

Do not start the next phase until the current phase has been
reviewed/merged and I explicitly say:

"Sonraki aşamaya geç."

Never combine two phases in the same push.

Never silently modify unrelated code.

Never remove a security control just to make a test pass.

If a new Critical or High issue is discovered outside the phase,
report it but do not expand the scope without approval.
```

Böylece Git history sonunda kabaca şöyle görünür:

```text
security(phase-00): establish security baseline
security(phase-01): harden docker network and service exposure
security(phase-02): enforce secure secrets and configuration defaults
security(phase-03): enforce secure email transport
security(phase-04): harden password reset workflow
security(phase-05): minimize sensitive transaction response data
security(phase-06): strengthen authorization regression coverage
security(phase-07): centralize strict request validation
security(phase-08): standardize secure API error handling
security(phase-09): harden rate limiting and abuse protection
security(phase-10): harden session jwt csrf and cors controls
security(phase-11): enforce database least privilege and hardening
security(phase-12): strengthen payment and ledger integrity
security(phase-13): add step-up authentication foundation
security(phase-14): bind transaction authorization to payment details
security(phase-15): add structured security audit logging
security(phase-16): add detection and incident response capabilities
security(phase-17): establish final devsecops and security regression gate
```

Bu şekilde **18 kontrollü push** ile ilerleriz. Özellikle Phase 01–05 mevcut gördüğümüz problemlere doğrudan müdahale ediyor; Phase 06–12 mevcut iyi güvenlik mekanizmalarını doğrulayıp sağlamlaştırıyor; Phase 13–17 ise projeyi demo seviyesinden production-security mimarisine doğru taşıyor.
