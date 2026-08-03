# SEPA Demo Banking

A simulation-only banking application built with Go, PostgreSQL and Next.js. It models German IBANs, SEPA-style payments, instant and scheduled transfers, standing orders, Verification of Payee (VoP), SSE updates and an atomic double-entry ledger.

**Author and maintainer:** Mustafa Özdemir

> **Demo-Banking – kein echtes Bankkonto.** This project does not connect to SEPA, PSD2, a bank, or a payment provider. It never moves real money and must not be presented as regulated, BaFin-approved, or PSD2-compliant banking software.

## What is included

- German-language responsive banking UI: Übersicht, Konten, Umsätze, Überweisen, Terminüberweisungen, Daueraufträge, Empfänger, Profil und Sicherheit.
- Six-step payment wizard with explicit demo consent.
- German-format demo IBAN generation with MOD-97 validation and a database uniqueness constraint.
- Umbuchung, internal demo transfer, simulated SEPA, simulated SEPA Instant, scheduled payments and standing orders.
- Demo VoP results: `MATCH`, `CLOSE_MATCH`, `NO_MATCH`, and `OTHER`; mismatch decisions are audited.
- Payment state machine: `DRAFT`, `AWAITING_CONFIRMATION`, `SCHEDULED`, `PROCESSING`, `BOOKED`, `FAILED`, `CANCELLED`.
- Idempotent payment creation through `Idempotency-Key`.
- Exactly one debit and one credit for every booked payment, inside one PostgreSQL transaction.
- External payments balance against a system settlement account.
- PostgreSQL worker claims due jobs with `FOR UPDATE SKIP LOCKED`; a standalone worker binary is also provided.
- SSE dashboard refresh with polling fallback.

Demo IBANs use bank code `99999999`. They have a valid checksum for testing, but the code is intentionally not routable and must never be submitted to a real payment network.

## Architecture

```text
Browser / Next.js :3000
        |
        | HTTP + HttpOnly cookie + SSE
        v
Go / Chi API :8080 ---- in-process scheduler (free demo profile)
        |
        +---- optional payment-worker (paid reliable profile)
        |
        v
PostgreSQL ---- payment orders + immutable double-entry entries + audit events
```

Money is represented as a JSON string at the API boundary, `decimal` in Go, and `NUMERIC` in PostgreSQL. Existing demo USD values are relabelled as EUR by migration `000005`; no real foreign-exchange conversion is claimed or performed.

## Run locally

Requirements: Docker Desktop and Docker Compose.

```bash
cp .env.example .env
# Set a unique JWT_SECRET in .env
docker compose up --build
```

Open [http://localhost:3000](http://localhost:3000). API health is available at [http://localhost:8080/health](http://localhost:8080/health), and Swagger UI at [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html).

To load fictional, repeat-safe demo data:

```env
DEMO_SEED=true
DEMO_SEED_PASSWORD=<unique-secret-with-at-least-15-bytes>
```

The seed creates the fictional users `anna.beispiel@demo.invalid` and
`max.mustermann@demo.invalid`; both use the deployment secret supplied through
`DEMO_SEED_PASSWORD`. Never reuse that secret for a real account.

The `.invalid` domain and all names, balances and transactions are fictional. Set `DEMO_SEED=false` outside an isolated demo.

## Development and verification

Backend:

```bash
cd backend
go test ./...
go vet ./...
```

Database integration tests use `TEST_DB_URL`. With the Compose database:

```bash
TEST_DB_URL='postgresql://root:secret@localhost:5433/simple_ledger?sslmode=disable' go test ./...
```

Frontend:

```bash
cd frontend
yarn install --frozen-lockfile
yarn type-check
yarn lint
yarn build
```

Regenerate sqlc and OpenAPI after changing their sources:

```bash
cd backend
sqlc generate
go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/main.go -o docs
```

Migrations are additive and live in `backend/postgres/migrations`. The container entrypoint runs them before the API starts and exits on a non-transient migration failure.

## Payment API

Authenticated additions include:

- `POST /payees/verify`
- `POST|GET /payments`
- `GET /payments/{id}`
- `POST /payments/{id}/confirm`
- `POST /payments/{id}/cancel`
- `POST|GET /standing-orders`
- `PATCH|DELETE /standing-orders/{id}`
- `GET /accounts/{id}/transactions`
- `GET /beneficiaries`
- `GET /events` (SSE)

`POST /payments` requires a unique `Idempotency-Key` header. Reusing the key with the same normalized intent returns the original order; reusing it with changed payment fields returns a conflict. Creating an order does not move funds. The client must run VoP, show the summary, and explicitly call the confirmation endpoint.

This is intentionally a demonstration of payment orchestration, not real Strong Customer Authentication or a TAN implementation.

## Render deployment profiles

The default [`render.yaml`](render.yaml) keeps the existing Frankfurt region and deploys free web services plus PostgreSQL. Secrets are generated or injected by Render and are not stored in the repository.

### 1. Free demo

`ENABLE_IN_PROCESS_SCHEDULER=true` runs the scheduler inside the API. Free Render web services can sleep while idle, so scheduled payments are processed only when the service is active and **have no exact execution-time guarantee**.

### 2. Reliable scheduling

Use a paid background worker:

1. Merge the service in [`render.worker.example.yaml`](render.worker.example.yaml) into the Blueprint or create the equivalent worker in Render.
2. Set `ENABLE_IN_PROCESS_SCHEDULER=false` on the web service.
3. Keep the worker command `/usr/local/bin/payment-worker` and point `DB_URL` to the same PostgreSQL database.
4. Keep `RUN_DB_MIGRATIONS_ON_START=false` on the worker; the web service performs migrations.

The sample does not silently enable a paid service. Review Render's current [free service limits](https://render.com/docs/free) before deployment: free services can spin down, free Postgres has lifecycle and backup limitations, and those conditions can change. Blueprint fields are documented in Render's [Blueprint specification](https://render.com/docs/blueprint-spec).

## Security boundaries

- Source-account ownership is checked on the backend for every payment.
- Secure/HttpOnly/SameSite cookie handling, CSRF checks, CORS allowlists and endpoint rate limits remain enforced.
- Account and payment list responses mask beneficiary IBANs; a full IBAN is returned only for owner-authorized detail flows.
- Strict JSON decoding rejects unknown fields and reduces mass-assignment risk.
- Stable account lock ordering and transactional writes avoid partial ledger entries and reduce deadlocks.
- Logs and audit data must not contain JWTs, passwords, database credentials, or unmasked personal data.

For the rules represented by this simulation, consult current official material from the [ECB on instant payments and VoP](https://www.ecb.europa.eu/paym/retail/instant_payments/html/instant_payments_regulation.en.html), the [European Payments Council on SCT](https://www.europeanpaymentscouncil.eu/what-we-do/sepa-credit-transfer), and the [Deutsche Bundesbank SEPA guidance](https://www.bundesbank.de/dynamic/action/de/aufgaben/unbarer-zahlungsverkehr/serviceangebot/sepa/613964/fragen-und-antworten-zu-sepa). This code is not legal, compliance, or operational banking advice.
