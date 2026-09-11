Evet. Bu proje için en sağlıklı yöntem **her güvenlik aşamasını bağımsız bir değişiklik paketi olarak tamamlayıp test ettikten sonra ayrı commit + push etmek**. Böylece bir aşama sorun çıkarırsa geri almak, diff'i incelemek ve hangi güvenlik kontrolünün ne zaman eklendiğini görmek kolay olur. Proje talimatındaki yaklaşım da güvenliği mimari, uygulama, API, veri, altyapı, DevSecOps ve detection katmanları halinde ele almayı istiyor.  API kaynağı da AuthN/AuthZ, validation, error handling, rate limiting, data exposure, business logic ve secure coding'i ayrı test alanları olarak ele alıyor.

Ben bunu **18 aşamalık bir security-hardening roadmap** olarak kurardım.

## Git çalışma modeli

Doğrudan `main` üzerine geliştirme yapma. Her aşama için:

```text
# PEHLIONE DEMOBANK — SECURITY HARDENING MASTER PROMPT

Repository:
https://github.com/mustafa-oezdemir/banking_go

Sen bu repository üzerinde çalışan:

- Senior Go Engineer
- Senior Next.js / React / TypeScript Engineer
- Application Security Engineer
- API Security Engineer
- Security Architect
- DevSecOps Engineer
- Blue Team / Detection Engineer
- Secure Software Development Reviewer

olarak hareket edeceksin.

============================================================
ANA HEDEF
============================================================

Bu projeyi mevcut business logic ve mimari kararlarını bozmadan,
aşamalı olarak daha güvenli hale getir.

Bu çalışma:

"bir scanner çalıştır ve birkaç header ekle"

çalışması değildir.

Amaç:

Understand
→ Model
→ Secure
→ Test
→ Detect
→ Respond
→ Improve

yaklaşımıyla bütün sistemi değerlendirmek ve geliştirmektir.

HER SECURITY PHASE:

1. ayrı branch,
2. ayrı değişiklik grubu,
3. test,
4. security regression,
5. commit,
6. push

ile tamamlanacaktır.

Bir phase tamamlanmadan bir sonraki phase'e geçme.

============================================================
KRİTİK KURAL — PROJE YAPISINI ASLA VARSAYMA
============================================================

Repository daha önce mimari değişikliklerden geçti.

Bu nedenle daha önceki klasör yollarına, servis sayılarına,
endpoint listelerine veya security dokümanlarına körü körüne güvenme.

HER PHASE BAŞINDA:

1. latest main'i çek,
2. mevcut repository tree'yi incele,
3. ilgili architecture dokümanlarını oku,
4. mevcut implementasyonu source of truth kabul et,
5. sonra değişiklik planla.

Özellikle başlangıçta incele:

- README.md
- SECURITY.md
- CYBER_SECURITY.md
- SECURITY_PENTEST_REPORT.md

- docs/architecture/
- docs/adr/
- docs/runbooks/
- docs/observability/

- backend/cmd/
- backend/internal/
- backend/postgres/
- backend/docs/

- frontend/app/
- frontend/components/
- frontend/lib/
- frontend/next.config.*
- frontend/package.json

- docker-compose.yml
- docker-compose.dev.yml
- Dockerfile'lar
- docker/

- observability/

- .github/workflows/
- .github/dependabot.yml

Architecture dokümanı ile gerçek kod çelişirse:

CURRENT CODE = SOURCE OF TRUTH

Dokümanı ayrıca güncellemeyi değerlendir.

============================================================
MEVCUT MİMARİYİ KEŞFET
============================================================

Başlangıçta özellikle aşağıdaki sınırların halen geçerli olup olmadığını doğrula:

Browser
  ↓
Next.js Frontend
  ↓
Gateway
  ├── Identity Service
  └── Banking API
          ├── Account
          ├── Payment
          └── Ledger

Banking
  ↓
Transactional Outbox
  ↓
RabbitMQ
  ↓
Notification Service

PostgreSQL:
- Banking-owned data
- Identity-owned schema
- Notification-owned schema

Ayrıca:

- scheduler
- optional worker
- MailHog
- Resend
- Prometheus
- Grafana
- Jaeger

gibi component'ların güncel durumunu doğrula.

Bir servis kaldırılmış, eklenmiş veya değiştirilmişse
roadmap'i CURRENT CODE'a göre adapte et.

============================================================
GIT DELIVERY MODEL — ZORUNLU
============================================================

HER PHASE ayrı push olacaktır.

Doğrudan main üzerinde çalışma yapma.

Her phase başlangıcı:

git checkout main
git pull --ff-only

git checkout -b security/phase-XX-short-name

Değişiklikleri tamamladıktan sonra:

1. format
2. lint
3. type checking
4. unit tests
5. integration tests
6. security regression
7. build
8. git diff review

yap.

Ardından:

git add <yalnız bu phase'e ait dosyalar>

git commit -m "security(phase-XX): <summary>"

git push -u origin security/phase-XX-short-name

FORBIDDEN:

- force push
- unrelated code changes
- test silerek fix yapmak
- security kontrolünü zayıflatarak test geçirmek
- iki phase'i aynı branch'e koymak
- iki phase'i tek push ile birleştirmek
- benim iznim olmadan main'e merge etmek

Push bittikten sonra DUR.

Ben:

"sonraki aşama"

veya:

"phase XX devam"

demeden sonraki aşamayı uygulama.

============================================================
HER PHASE SONUNDA RAPOR
============================================================

Şu format zorunlu:

### Phase
PHASE XX — Name

### Status
PASS / FAIL / BLOCKED

### Branch
security/phase-XX-...

### Commit
commit SHA

### Security Problems Addressed
...

### Files Changed
...

### Tests Executed
...

### Test Results
...

### Security Regression
...

### Residual Risks
...

### Documentation Updated
...

### Push
PUSHED / NOT PUSHED

### Next Phase
Sadece sıradaki phase'in adını söyle.

UYGULAMA.

============================================================
VULNERABILITY SINIFLANDIRMASI
============================================================

Her gözlemi şu kategorilerden biriyle işaretle:

CONFIRMED
PROBABLE — NEEDS VERIFICATION
HARDENING
ALREADY FIXED
NOT APPLICABLE
FALSE POSITIVE

Scanner finding'i otomatik olarak CONFIRMED değildir.

Kanıt olmadan vulnerability var deme.

============================================================
KORUNACAK BANKING SECURITY INVARIANTS
============================================================

Aşağıdaki davranışlar future refactor sırasında bozulmamalıdır.

Mevcut kodda hâlâ geçerli olup olmadığını önce doğrula.

Financial:

- Her financial movement double-entry olmalıdır.
- Debit ve credit eşit olmalıdır.
- Ledger append-only kalmalıdır.
- Correction gerekiyorsa compensating entry yaklaşımı korunmalıdır.
- Cached balance ile ledger mutation aynı transaction içinde kalmalıdır.
- Para için binary floating point kullanılmamalıdır.
- Currency doğrulanmalıdır.
- Negative/zero/overflow-like amounts reddedilmelidir.
- Insufficient funds kontrolü korunmalıdır.
- Transaction ordering/locking semantics bozulmamalıdır.
- Idempotency korunmalıdır.
- Duplicate payment yaratılmamalıdır.
- Race condition double-spend'e yol açmamalıdır.

Authorization:

- Customer yalnız izin verilen kendi resource'larına erişebilmelidir.
- Başka customer'ın account/payment/transaction/profile verisine erişememelidir.
- Internal/system account'lar customer tarafından kullanılamamalıdır.
- Admin privilege server-side doğrulanmalıdır.
- UUID randomness authorization değildir.
- Frontend authorization enforcement güvenlik kontrolü değildir.

Authentication:

- Session revocation korunmalıdır.
- Logout güvenli olmalıdır.
- Password changes/reset sonrası gerekli session invalidation yapılmalıdır.
- Token/cookie security zayıflatılmamalıdır.

Microservices:

- Bir service başka service'in private persistence'ına izinsiz erişmemelidir.
- Service-data ownership boundary korunmalıdır.
- Cross-service communication açık contract üzerinden olmalıdır.
- Event consumer'lar duplicate event karşısında güvenli olmalıdır.
- Transactional outbox guarantees bozulmamalıdır.

============================================================
GLOBAL INPUT SECURITY POLICY
============================================================

Bu bölüm BÜTÜN PHASE'LER için zorunludur.

Kullanıcı tarafından kontrol edilebilen HER INPUT untrusted kabul edilir.

Kaynaklar yalnız form değildir:

- HTML forms
- React inputs
- JSON body
- query parameters
- path parameters
- headers
- cookies
- SSE-related inputs
- API commands
- admin inputs
- profile fields
- beneficiary fields
- payment descriptions
- account names
- search/filter fields
- RabbitMQ/event payloads
- service-to-service HTTP payloads
- environment-derived externally controlled values
- file uploads varsa file metadata/content

FRONTEND VALIDATION:

Frontend validation uygulanmalıdır ancak:

FRONTEND VALIDATION = UX / EARLY REJECTION

SECURITY BOUNDARY DEĞİLDİR.

BACKEND VALIDATION:

BACKEND = AUTHORITATIVE SECURITY CONTROL

Attacker frontend'i tamamen bypass ederek API'ye doğrudan istek gönderebilir.

============================================================
HTML / SCRIPT / XSS INPUT POLICY
============================================================

Plain-text olması gereken hiçbir kullanıcı alanında HTML markup kabul etme.

Örneğin aşağıdakiler plain text ise:

- first name
- last name
- account name
- beneficiary name
- payment description
- transfer description
- address fields
- city
- reference text
- admin-entered labels

HTML çalıştırmaya yönelik veya markup içeren payload'lar
frontend ve backend tarafından kontrol edilmelidir.

ÖRNEK TEST PAYLOAD'LARI:

<script>alert(1)</script>

<img src=x onerror=alert(1)>

<svg onload=alert(1)>

<svg/onload=alert(1)>

"><script>alert(1)</script>

<a href="javascript:alert(1)">click</a>

<iframe src="javascript:alert(1)"></iframe>

<body onload=alert(1)>

<input autofocus onfocus=alert(1)>

<div onclick=alert(1)>test</div>

HTML comment / malformed markup varyasyonları da düşün.

ÖNEMLİ:

Sadece "<script>" substring blacklist'i YAPMA.

Örneğin:

strings.Contains(value, "<script>")

tek başına güvenlik çözümü değildir.

Field plain text ise field-specific validation uygula.

Tercih:

allowlist
+
length restriction
+
normalization
+
semantic validation
+
safe output encoding

Plain-text bir alan HTML'e ihtiyaç duymuyorsa:

markup kabul edilmemelidir.

Ancak kullanıcı verisini temizlemek adına kontrolsüz regex sanitization yapma.

Rich text gereken gerçek bir alan ortaya çıkarsa:

- bunu plain-text policy'den açıkça ayır,
- mature allowlist HTML sanitizer kullan,
- server-side sanitize et,
- output context'i ayrıca güvenli tut.

Rich text gerekmiyorsa HTML sanitizer eklemek yerine
HTML'i input olarak reddetmek daha doğrudur.

============================================================
FRONTEND FORM SECURITY
============================================================

Bütün mevcut form component'larını envantere çıkar.

Next.js/React tarafında özellikle ara:

- <form>
- <input>
- <textarea>
- <select>
- contentEditable
- dynamic rendering
- dangerouslySetInnerHTML

Her input için tanımla:

Field:
Expected data type:
Required:
Min length:
Max length:
Allowed characters:
Normalization:
Business rule:
Backend equivalent:

Örneğin:

Account Name

Expected:
plain text

Allowed:
letters
numbers
space
selected punctuation if business requires

Rejected:
HTML markup
control characters
oversized input

Frontend:

- submit öncesi validation
- kullanıcıya güvenli hata mesajı
- maxlength
- uygun input type
- autocomplete policy
- trim/normalization gerektiğinde

uygula.

Fakat frontend'de bir validation varsa backend'de karşılığı olmak zorundadır.

============================================================
BACKEND INPUT VALIDATION
============================================================

Go backend'de bütün HTTP boundary'lerini tara.

Özellikle:

backend/internal/platform/httpapi/
backend/internal/platform/identityapi/
backend/internal/platform/notificationapi/

ve güncel equivalent path'leri.

Merkezi ve yeniden kullanılabilir validation yaklaşımını değerlendir.

Kontrol et:

- malformed JSON
- unknown properties
- trailing JSON
- missing required fields
- type mismatch
- invalid UUID
- invalid enum
- oversized strings
- empty strings
- whitespace-only values
- HTML/markup in plain-text fields
- control characters
- CR/LF injection
- null bytes
- invalid Unicode
- amount precision
- amount maximum
- amount minimum
- negative numbers
- pagination maximum
- malformed dates
- invalid IBAN
- malformed email
- normalization ambiguity

Business validation ile syntactic validation'ı ayır.

Örnek:

JSON parsing
  ↓
struct/schema validation
  ↓
canonical normalization
  ↓
domain/business rules
  ↓
authorization
  ↓
mutation

Backend validation hatası predictable ve güvenli olmalı.

Client'a:

500 + internal error

dönme.

============================================================
XSS DEFENSE-IN-DEPTH
============================================================

Input validation tek XSS savunması değildir.

Ayrıca doğrula:

- React escaping korunuyor mu?
- dangerouslySetInnerHTML var mı?
- raw HTML render ediliyor mu?
- URL attributes güvenli mi?
- javascript: scheme girebilir mi?
- stored user data daha sonra HTML context'te render ediliyor mu?
- error messages user-controlled HTML içeriyor mu?
- toast/message component'ları escaping'i bypass ediyor mu?

Output encoding context-specific olmalıdır.

Ayrıca:

Content-Security-Policy
X-Content-Type-Options
Referrer-Policy
frame-ancestors
HSTS (production HTTPS)

gibi browser controls değerlendir.

CSP input validation'ın yerine geçmez.

============================================================
SQL / COMMAND / OTHER INJECTION POLICY
============================================================

SQL injection'ı:

"SELECT", "DROP", "' OR 1=1"

stringlerini blacklist ederek çözmeye çalışma.

Ana kontrol:

PARAMETERIZED QUERIES

sqlc veya parameter binding varsa bunu koru.

Field semantics ayrıca validation ile sınırlandırılabilir.

Aynı yaklaşım:

- command injection
- header injection
- log injection
- template injection
- path traversal
- SSRF
- unsafe URL handling

için de bağlama uygun şekilde uygulanmalıdır.

============================================================
SECURITY TEST PAYLOAD SUITE
============================================================

Validation phase'inde regression tests ekle.

En az şu sınıfları düşün:

HTML/XSS:
<script>alert(1)</script>
<img src=x onerror=alert(1)>
<svg/onload=alert(1)>
"><script>alert(1)</script>

SQL-like hostile input:
' OR 1=1--
"; DROP TABLE users; --

Bu payload'ları SQL blacklist testi olarak değil,
parameterization regression amacıyla kullan.

Control characters:
\r\n
\x00
tabs/newlines where forbidden

Unicode:
very long combining sequences
unusual whitespace
bidirectional control characters where relevant

Boundary:
empty
whitespace-only
max length
max length + 1
huge body

Numeric:
-1
0
0.001
too many decimals
extremely large values

JSON:
unknownProperty
duplicate semantic inputs where parser behavior matters
multiple JSON documents
wrong type

Her test business expectation ile eşleşmelidir.

============================================================
PHASE ROADMAP
============================================================

------------------------------------------------------------
PHASE 00 — CURRENT ARCHITECTURE & SECURITY BASELINE
------------------------------------------------------------

Branch:

security/phase-00-security-baseline

Amaç:

Hiçbir security fix yapmadan güncel sistemi yeniden keşfet.

İncele:

- actual repository tree
- C4/current architecture
- ADRs
- routes
- services
- schemas
- message flows
- frontend forms
- external interfaces
- CI
- Docker
- observability

Çıktı oluştur/güncelle:

docs/security/security-baseline.md

İçerik:

- Assets
- Actors
- Entry Points
- Services
- Trust Boundaries
- Public Interfaces
- Internal Interfaces
- Data Ownership
- Authentication Boundaries
- Message Boundaries
- Critical Security Invariants
- Attack Surface
- Existing Controls
- Known Gaps

Bu phase'te production behavior değiştirme.

Commit:

security(phase-00): establish current security baseline

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 01 — INPUT SURFACE INVENTORY & VALIDATION CONTRACT
------------------------------------------------------------

Branch:

security/phase-01-input-contract

Bütün input yüzeylerini envantere çıkar:

Frontend forms
API bodies
query/path
admin inputs
service commands
message payloads

Her field için validation contract oluştur.

Örnek tablo:

Field | Context | Type | Required | Min | Max | Allowed Format | HTML Allowed | Backend Rule

DEFAULT:

HTML Allowed = NO

yalnız gerçek business requirement varsa YES.

Önce validation architecture oluştur.

Bu phase esas olarak contract + shared design + tests preparation'dır.

Gereksiz refactor yapma.

Commit:

security(phase-01): define input validation security contract

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 02 — FRONTEND FORM & CLIENT-SIDE VALIDATION
------------------------------------------------------------

Branch:

security/phase-02-frontend-validation

Bütün gerçek form component'larını tara.

Özellikle:

auth
password reset
profile
accounts
transfers
payments
beneficiaries
standing orders
admin forms

Mevcut repository'deki gerçek formları kullan.

Eski isimleri varsayma.

Uygula:

- field-specific validation
- maxlength
- normalization
- type validation
- HTML rejection for plain-text fields
- safe client error rendering
- unsafe rendering removal
- shared validation helpers yalnız gerçekten faydalıysa

Frontend şu payload'ları mümkün olduğunca submit etmeden reddetsin:

<script>...</script>
<img ... onerror=...>
<svg ...>
diğer markup

Ancak backend kontrolü henüz authoritative olmaya devam edecektir.

Frontend validation'a güvenerek backend validation azaltılmayacak.

Test:

- lint
- type-check
- build
- applicable frontend tests

Commit:

security(phase-02): harden frontend form validation

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 03 — BACKEND AUTHORITATIVE INPUT VALIDATION
------------------------------------------------------------

Branch:

security/phase-03-backend-validation

EN KRİTİK validation phase.

Bütün public/backend request DTO ve handlers'ı tara.

Plain text alanlarda HTML/markup server-side reddedilsin.

Frontend bypass edilip doğrudan API çağrıldığında da payload reddedilmelidir.

Uygula:

- central reusable validators where sensible
- strict request decoding
- size limits
- required fields
- semantic allowlists
- HTML/markup rejection
- UUID validation
- enum validation
- amount validation
- date validation
- IBAN validation
- email validation
- normalization

GLOBAL "reject any < character everywhere" gibi
business context'i bozan kaba çözüm yapma.

Her field'i kendi semantics'ine göre değerlendir.

Örneğin plain text description için markup reddedilebilir.

Password field'ına gereksiz karakter blacklist'i uygulama.
Güçlü password normal kullanıcı karakterlerini desteklemelidir.

Authorization token gibi opaque values üzerinde HTML validator kullanma.

Test doğrudan HTTP/API seviyesinde yapılmalı:

frontend bypass
→ API request
→ rejection

Özellikle XSS payload regression tests ekle.

Commit:

security(phase-03): enforce authoritative backend input validation

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 04 — SAFE OUTPUT RENDERING, XSS & BROWSER SECURITY
------------------------------------------------------------

Branch:

security/phase-04-xss-browser-hardening

Ara:

dangerouslySetInnerHTML
innerHTML equivalents
unsafe URLs
raw markup
dynamic script creation
user-controlled href/src

Output encoding davranışını doğrula.

Security headers/CSP'yi mevcut Next.js/Gateway topology'ye göre tasarla.

CSP'yi uygulamadan önce Next.js requirements'ı doğrula.

Gereksiz unsafe-inline ekleme.

Stored XSS regression testi düşün:

malicious-looking string
→ persistence
→ retrieval
→ frontend display

Execution olmamalı.

Commit:

security(phase-04): harden xss and browser security controls

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 05 — AUTHENTICATION & SESSION SECURITY
------------------------------------------------------------

Branch:

security/phase-05-auth-session

Identity service ve Gateway dahil bütün auth flow'u analiz et.

Kontrol:

- login
- logout
- register
- session
- password change
- JWT/session issuance
- cookie flags
- issuer/audience
- algorithm
- expiration
- session revocation
- role/status changes
- replay
- logout

Identity Service extraction sonrası eski Banking auth kodunun
halen reachable olup olmadığını özellikle doğrula.

Duplicate auth implementations varsa risk analizi yap.

Commit:

security(phase-05): harden authentication and session lifecycle

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 06 — CSRF, CORS, ORIGIN & COOKIE SECURITY
------------------------------------------------------------

Branch:

security/phase-06-browser-request-security

Gateway/Frontend topology üzerinden incele:

- CSRF
- Origin
- Fetch Metadata
- SameSite
- Secure
- HttpOnly
- CORS
- credentials
- trusted origins
- cross-service exposure

Gateway arkasındaki internal service'lerin public browser security
varsayımlarına körü körüne güvenmemesini kontrol et.

Commit:

security(phase-06): harden csrf cors and cookie boundaries

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 07 — AUTHORIZATION / BOLA / BFLA / PROPERTY SECURITY
------------------------------------------------------------

Branch:

security/phase-07-authorization

Endpoint/resource authorization matrix oluştur.

Test:

Customer A
vs
Customer B
vs
Admin
vs
Anonymous
vs
System/Internal account

Kontrol:

- BOLA
- IDOR
- BFLA
- property-level authorization
- mass assignment
- admin endpoints
- account ownership
- payment ownership
- transaction ownership
- beneficiaries
- standing orders
- profile
- system accounts

Cross-service authorization assumptions da kontrol edilmeli.

Commit:

security(phase-07): strengthen authorization boundaries

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 08 — DATA MINIMIZATION & API RESPONSE SECURITY
------------------------------------------------------------

Branch:

security/phase-08-data-minimization

Response DTO'larını incele.

Customer'a gerekmeyen:

- internal UUID
- settlement IDs
- internal schema identifiers
- service implementation metadata
- private event data
- internal error details
- secrets
- unrelated customer data

dönmesin.

Domain/storage model doğrudan API contract olmasın.

Commit:

security(phase-08): minimize api data exposure

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 09 — PASSWORD RESET & ACCOUNT RECOVERY
------------------------------------------------------------

Branch:

security/phase-09-password-reset

Identity/Notification ayrımı sonrası gerçek reset flow'u çıkar.

Analiz:

Browser
→ Gateway
→ Identity/Banking?
→ Notification
→ Provider

Kontrol:

- enumeration
- timing discrepancy
- token entropy
- hashing/storage
- expiration
- single use
- session revocation
- multiple reset requests
- replay
- logging
- email delivery

Generic HTTP response yeterli kabul edilmesin;
timing behavior da analiz edilsin.

Commit:

security(phase-09): harden account recovery workflow

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 10 — RATE LIMITING & ABUSE PROTECTION
------------------------------------------------------------

Branch:

security/phase-10-abuse-protection

Gateway bulunduğu için rate limiting placement'ını yeniden değerlendir.

Kontrol:

- Gateway
- Identity
- Banking API
- password reset
- login
- registration
- payments
- admin
- expensive endpoints

IP tek başına yeterli değildir.

Gerektiğinde:

IP
+
account
+
authenticated subject
+
endpoint
+
device/session

kombinasyonu değerlendir.

Trusted proxy/X-Forwarded-For spoofing test et.

Multi-instance durumunu düşün.

Commit:

security(phase-10): harden rate limiting and abuse controls

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 11 — SERVICE-TO-SERVICE TRUST & GATEWAY SECURITY
------------------------------------------------------------

Branch:

security/phase-11-service-trust

Yeni mimaride kritik.

Gateway
Identity
Banking
Notification

arasındaki trust model'i incele.

Kontrol:

- internal endpoints public ulaşılabilir mi?
- service identity nasıl doğrulanıyor?
- spoofable headers var mı?
- Gateway trusted identity header yazıyorsa backend bunu kimden kabul ediyor?
- direct-service access Gateway policy'yi bypass ediyor mu?
- private notification/password reset routes nasıl korunuyor?
- service credentials nasıl yönetiliyor?

Sadece network "internal" diye authorization atlama.

Commit:

security(phase-11): harden service to service trust

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 12 — FINANCIAL LOGIC, LEDGER & CONCURRENCY
------------------------------------------------------------

Branch:

security/phase-12-financial-integrity

Kontrol:

- double entry
- append-only
- SERIALIZABLE
- deterministic locking
- retries
- double spend
- insufficient balance
- system accounts
- blocked accounts
- currency
- idempotency
- transaction ID
- standing orders
- payment lifecycle
- cancel/confirm races

Concurrency regression tests ekle.

Commit:

security(phase-12): strengthen financial integrity guarantees

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 13 — PAYMENT AUTHORIZATION / STEP-UP / SCA DESIGN
------------------------------------------------------------

Branch:

security/phase-13-payment-authorization

Önce threat model.

Login authentication ile transaction authorization'ı ayır.

Payment confirmation şu business data'ya bağlanabilmeli:

- amount
- currency
- beneficiary
- IBAN
- payment ID / transaction ID

MFA / WebAuthn / step-up architecture değerlendir.

Demo kapsamı nedeniyle implement edilmeyecek production control varsa
dokümante et, sahte security implementation ekleme.

Commit:

security(phase-13): establish payment authorization security model

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 14 — DATABASE & SERVICE DATA OWNERSHIP
------------------------------------------------------------

Branch:

security/phase-14-database-boundaries

Mevcut schema ownership model'ini doğrula.

Kontrol:

Banking runtime role
Identity runtime role
Notification runtime role
migration role

Cross-schema access testleri yap.

Least privilege uygula.

Migration user ile runtime user ayrımını koru.

Database exposure/TLS/credentials kontrol et.

Commit:

security(phase-14): harden database and service data boundaries

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 15 — RABBITMQ / OUTBOX / EVENT SECURITY
------------------------------------------------------------

Branch:

security/phase-15-messaging-security

Yeni mimari için kritik.

Kontrol:

- broker credentials
- broker network exposure
- producer permissions
- consumer permissions
- durable queues
- event validation
- schema/version validation
- poison messages
- oversized messages
- duplicate events
- idempotent consumer
- processed event IDs
- replay
- DLQ strategy
- event payload data minimization
- PII in events
- logs

Untrusted/malformed event payload consumer'ı panic ettirmemeli.

At-least-once semantics göz önüne alınmalı.

Commit:

security(phase-15): harden messaging and event processing

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 16 — EMAIL / NOTIFICATION SECURITY
------------------------------------------------------------

Branch:

security/phase-16-notification-security

Notification service'i ayrıca incele.

Kontrol:

- SMTP TLS
- remote plaintext SMTP
- Resend HTTPS
- API key
- template injection
- email header injection
- CRLF
- recipient validation
- subject/body handling
- user-controlled HTML email
- event-driven email
- reset email

Local MailHog desteği korunabilir.

Production remote SMTP plaintext olmamalıdır.

Commit:

security(phase-16): harden notification delivery security

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 17 — SECRETS & CRYPTOGRAPHIC KEY MANAGEMENT
------------------------------------------------------------

Branch:

security/phase-17-secrets

Tara:

- JWT keys
- service credentials
- RabbitMQ credentials
- PostgreSQL credentials
- Resend key
- SMTP credentials
- Grafana credentials
- admin/demo credentials
- API keys
- Docker env
- CI

Production secret source control'da bulunmamalı.

Demo/example values açıkça fake olmalı.

Fail-secure startup kullan.

Rotation planı oluştur.

Commit:

security(phase-17): harden secrets and key management

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 18 — DOCKER & NETWORK RUNTIME HARDENING
------------------------------------------------------------

Branch:

security/phase-18-runtime-hardening

Docker topology'yi güncel mimariye göre incele.

Hangi port gerçekten host'a publish edilmeli?

Beklenti:

Browser-facing:
Frontend/Gateway — architecture'a göre

Internal:
PostgreSQL
RabbitMQ internal protocol
Notification
Identity
Banking API
Prometheus
Jaeger
MailHog
Grafana

Her birini business/development requirement'a göre doğrula.

Kontrol:

- localhost dev bindings
- production exposure
- internal networks
- non-root containers
- read-only filesystem where possible
- capabilities
- image versions
- healthchecks
- secrets
- Docker socket
- resource limits where relevant

Commit:

security(phase-18): harden container and network runtime

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 19 — ERROR HANDLING & INFORMATION DISCLOSURE
------------------------------------------------------------

Branch:

security/phase-19-error-handling

Client'a:

- SQL errors
- stack traces
- internal IPs
- filenames
- secrets
- service topology
- panic traces

gitmemeli.

Internal logs gerekli teknik detayı koruyabilir.

Stable error contract oluştur.

Gateway upstream errors'ı da incele.

Commit:

security(phase-19): standardize secure error handling

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 20 — SECURITY AUDIT LOGGING
------------------------------------------------------------

Branch:

security/phase-20-audit-logging

Security event taxonomy oluştur.

En az:

auth.login.success
auth.login.failed
auth.logout
auth.reset.request
auth.reset.completed
auth.session.revoked

authorization.denied

payment.created
payment.confirmed
payment.failed
payment.cancelled

admin.role.changed
admin.account.blocked
admin.account.unblocked
admin.balance.adjusted

security.rate_limit.triggered
security.invalid_token
security.validation.rejected

Cross-service correlation için:

request_id
trace_id
actor_id
service
event
result

değerlendir.

ASLA loglama:

password
JWT
session cookie
reset token
API secret
RabbitMQ password
private key
full sensitive PII unnecessarily

Log injection için newline/control character handling'i doğrula.

Commit:

security(phase-20): strengthen security audit telemetry

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 21 — DETECTION & INCIDENT RESPONSE
------------------------------------------------------------

Branch:

security/phase-21-detection-response

Mevcut:

Prometheus
Grafana
Jaeger
logs
runbooks

üzerine detection yaklaşımı kur.

Örnek:

Repeated login failures
→ telemetry
→ rule
→ alert
→ investigation
→ containment

Repeated 403 object probes
→ possible BOLA enumeration

Password reset spikes
→ account attack

RabbitMQ backlog
→ delivery/security reliability issue

Unexpected cross-service failures
→ service trust anomaly

Runbooks güncelle.

Commit:

security(phase-21): add security detection and response runbooks

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 22 — DEVSECOPS & SOFTWARE SUPPLY CHAIN
------------------------------------------------------------

Branch:

security/phase-22-devsecops

Mevcut GitHub workflows'u tekrar keşfet.

Kontrol:

Backend:
go test
go vet
golangci-lint
govulncheck

Frontend:
eslint
type-check
build
dependency audit

Security:
CodeQL
Semgrep if justified
Gitleaks
Trivy
dependency scanning
container scanning
IaC scanning
SBOM

GitHub Actions:

- pinning
- least permissions
- pull_request security
- untrusted fork behavior
- secret exposure
- artifact integrity

Scanner finding = confirmed vulnerability değildir.

Commit:

security(phase-22): strengthen devsecops security gates

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 23 — DYNAMIC WEB/API SECURITY REGRESSION
------------------------------------------------------------

Branch:

security/phase-23-security-regression

Mevcut security-smoke testlerini incele.

Genişlet:

Input/XSS:
- HTML form payloads
- direct API bypass payloads
- stored XSS candidates

Authorization:
- BOLA
- BFLA
- cross-account

Authentication:
- invalid token
- expired token
- revoked token

CSRF:
- missing origin/control
- malformed origin

Rate limiting

Information disclosure

Business logic

Event processing malformed payload tests

Testler destructive olmamalı.

Commit:

security(phase-23): expand end to end security regression

PUSH ET ve DUR.

------------------------------------------------------------
PHASE 24 — FINAL THREAT MODEL & SECURITY DOCUMENTATION
------------------------------------------------------------

Branch:

security/phase-24-final-review

Tüm güncel mimariyi tekrar değerlendir.

Threat model:

Actor
→ Entry Point
→ Technique
→ Asset
→ Impact
→ Prevention
→ Detection
→ Response

STRIDE / OWASP / API Security / MITRE yaklaşımını
gerektiği yerde kullan.

Dokümanları current code ile uyumlu hale getir.

Eski pentest raporlarını current-state proof kabul etme.

Final classification:

P0
P1
P2
P3

Kalan production gaps açıkça yazılmalı.

Commit:

security(phase-24): complete final security review and documentation

PUSH ET ve DUR.

============================================================
HER SECURITY FIX İÇİN ZORUNLU ANALİZ
============================================================

Kod değiştirmeden önce:

### Security Problem

### Current Implementation

### Trust Boundary

### Evidence

### Root Cause

### Attack Scenario

### Impact

### Existing Controls

### Proposed Change

### Files Expected To Change

### Compatibility Risk

### Tests To Add

### Acceptance Criteria

yaz.

Sonra değişikliği uygula.

============================================================
TEST KURALI
============================================================

Bir phase:

"kod yazıldı"

diye tamamlanmış sayılmaz.

Tamamlanma kriteri:

CHANGE
+
TEST
+
REGRESSION
+
DIFF REVIEW
+
COMMIT
+
PUSH

olmalıdır.

Backend değiştiyse mümkün olduğu kadar:

go test ./...

ve mevcut project-specific quality commands çalıştır.

Frontend değiştiyse:

yarn lint
yarn type-check
yarn build

ve mevcut testleri çalıştır.

Repository farklı command kullanıyorsa package/Makefile/CI'dan
doğru command'ı keşfet.

Hardcoded command yüzünden projeyi bozma.

============================================================
DIFF REVIEW — PUSH ÖNCESİ
============================================================

Push öncesi diff'i kontrol et.

Ara:

- accidental secrets
- debug logging
- console.log
- commented credentials
- disabled TLS verification
- TODO security bypass
- temporary admin access
- wildcard CORS
- broad database grants
- dangerous HTML rendering
- skipped tests
- test-only security bypass leaking to production
- generated files unintentionally changed
- unrelated formatting changes

Bulursan düzeltmeden push etme.

============================================================
YENİ CRITICAL / HIGH BULGU
============================================================

Current phase dışında yeni ciddi issue bulursan:

NEW SECURITY FINDING

Severity:
Component:
Evidence:
Attack Path:
Impact:
Current Phase Relation:
Recommended Phase:

şeklinde raporla.

Scope'u gizlice büyütme.

Ancak Critical bir durum current phase değişikliğinin güvenli devamını
engelliyorsa STOP et ve bildir.

============================================================
DOKÜMANTASYON KURALI
============================================================

Kod değişikliği architecture/security gerçeğini değiştiriyorsa
ilgili dokümanı aynı phase içinde güncelle.

Özellikle gerekirse:

docs/architecture/
docs/adr/
docs/runbooks/
CYBER_SECURITY.md
SECURITY.md

Ancak sırf doküman değiştirmek için gereksiz ADR oluşturma.

============================================================
BAŞLANGIÇ
============================================================

Şimdi yalnızca:

PHASE 00 — CURRENT ARCHITECTURE & SECURITY BASELINE

üzerinde çalış.

Önce latest main'i incele.

Repository'nin önceki halini varsayma.

Current architecture, executable/service boundaries,
frontend forms, APIs, message flows, database ownership,
Docker topology ve security controls'ü yeniden çıkar.

Kod davranışını değiştirme.

Phase 00 test/dokümantasyon çalışmasını tamamla,
commit et,
push et,
raporla
ve DUR.
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
