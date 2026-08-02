# Double-Entry Bank Ledger in Go

[![CI](https://github.com/mustafa-oezdemir/banking_go/actions/workflows/ci.yml/badge.svg)](https://github.com/mustafa-oezdemir/banking_go/actions/workflows/ci.yml)
[![Docker](https://github.com/mustafa-oezdemir/banking_go/actions/workflows/docker.yml/badge.svg)](https://github.com/mustafa-oezdemir/banking_go/actions/workflows/docker.yml)
[![CodeQL](https://github.com/mustafa-oezdemir/banking_go/actions/workflows/codeql.yml/badge.svg)](https://github.com/mustafa-oezdemir/banking_go/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mustafa-oezdemir/banking_go)](https://goreportcard.com/report/github.com/mustafa-oezdemir/banking_go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Production-focused Go backend that models bank-style money movement using strict double-entry accounting.

It demonstrates:

- Atomic transactions with PostgreSQL
- Concurrency safety with serializable isolation + retry
- Ledger-based reconciliation
- JWT auth + account-level authorization
- API docs, health checks, and Dockerized deployment

The frontend is intentionally minimal—Next.js and TypeScript—so the focus stays on the Go backend. Its source is included in [`frontend/`](frontend/).

## Live Demo

- Frontend: https://golangbank.app
- Repository: https://github.com/mustafa-oezdemir/banking_go
- Frontend Source: https://github.com/mustafa-oezdemir/banking_go/tree/main/frontend
- API Docs: https://golangbank.app/swagger
- Health: https://golangbank.app/health

Dont forget to star and fork this project repo

## Article vs README

This README is intentionally concise and implementation-focused.

For the full technical narrative and tutorial, read the FreeCodeCamp article: [How to Build a Bank Ledger in Golang with PostgreSQL using Double-Entry Accounting](https://www.freecodecamp.org/news/build-a-bank-ledger-in-go-with-postgresql-using-the-double-entry-accounting-principle/)
![freecodecamp](backend/internal/public/freecodecamp.png)

## Core Ledger Model

Each money movement writes balanced entries into the `entries` table:

- deposit: credit user account, debit settlement account
- withdrawal: debit user account, credit settlement account
- transfer: debit source account, credit destination account

Key constraints and behaviors implemented in code:

- single-sided entry rows (debit xor credit)
- account row locking (`FOR UPDATE`) during balance-changing operations
- serializable transactions with automatic retry on SQLSTATE `40001`
- reconciliation query computes `SUM(credit) - SUM(debit)` as source of truth
  ![Demo](backend/internal/public/frontend.png)

## Tech Stack

- Go 1.24+
- Router: go-chi/chi
- Database: PostgreSQL 16
- Query layer: sqlc
- Auth: JWT (go-chi/jwtauth)
- Logging: zerolog
- API docs: swaggo + http-swagger
- Testing: Go test + testify + race detector
- Runtime: Docker + docker-compose

## API Endpoints

Public:

- `POST /register`
- `POST /login`
- `GET /health`
- `GET /swagger/index.html`

Protected (Bearer token required):

- `POST /accounts`
- `GET /accounts`
- `GET /accounts/{id}`
- `POST /accounts/{id}/deposit`
- `POST /accounts/{id}/withdraw`
- `POST /transfers`
- `GET /accounts/{id}/entries`
- `GET /accounts/{id}/reconcile`
- `GET /transactions/{id}`
  ![Backend API Endpoint; Swagger Documentation](backend/internal/public/swagger.png)

## Project Structure

```text
.
├── backend/
│   ├── cmd/
│   ├── internal/
│   ├── postgres/
│   ├── docs/
│   ├── Dockerfile
│   └── go.mod
├── frontend/
│   ├── app/
│   ├── components/
│   └── Dockerfile
├── docker-compose.yml
├── start.ps1
└── README.md
```

## Local Development

### Prerequisites

- Go 1.24+
- Node.js 22+ with Corepack/Yarn
- Docker + docker compose
- migrate CLI
- sqlc CLI
- swag CLI (only needed when regenerating docs)

Install tools:

```bash
go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/swaggo/swag/cmd/swag@latest
```

### Run Locally

#### One-command full stack (Windows)

Docker Desktop açıkken repository kökünden çalıştırın:

```powershell
.\start.ps1
```

This builds and starts the complete local stack:

- Frontend: http://localhost:3000 (`frontend/`)
- Backend API: http://localhost:8080 (`backend/`)
- PostgreSQL: `localhost:5433`

Stop all services with:

```powershell
.\start.ps1 -Stop
```

#### Backend only

```bash
git clone https://github.com/mustafa-oezdemir/banking_go.git
cd banking_go
cp .env.example .env
# Set JWT_SECRET to at least 32 characters: openssl rand -base64 32
cd backend

make postgres
make migrate-up
make sqlc
make server
```

Database configuration is split into explicit fields in `.env`:

- `DB_HOST` and `DB_PORT`: PostgreSQL network address
- `DB_NAME`: database name
- `DB_USER` and `DB_PASSWORD`: database credentials
- `DB_SSLMODE`: PostgreSQL TLS mode

`DB_URL` is still supported for managed platforms that provide a complete connection string.

Open:

- Swagger: http://localhost:8080/swagger/index.html
- Health: http://localhost:8080/health

## Testing

Recommended (requires Docker running):

```bash
make postgres
make test
```

With coverage report:

```bash
make coverage
```

Full CI-style run including migrations:

```bash
make ci-test
```

Environment used by tests:

- `TEST_DB_URL` (defaults to `postgresql://root:secret@localhost:5433/simple_ledger?sslmode=disable`)

## Make Targets

```bash
make postgres       # Start PostgreSQL container
make migrate-up     # Apply migrations
make migrate-down   # Rollback last migration
make sqlc           # Regenerate sqlc query code
make server         # Run the API server
make test           # Run tests with race detector
make coverage       # Generate coverage report
make lint           # Run golangci-lint
make ci-test        # Full test run including migrations
make docker-build   # Build Docker image locally
make docker-up      # Start full stack with Docker Compose
make docker-down    # Stop Docker Compose services
```

## Security

The application uses HttpOnly `SameSite=Strict` session cookies, CSRF and
origin checks, per-IP rate limits, request-size and JSON content-type limits,
security headers, parameterized SQLC queries, and owner checks on every account
operation. Passwords must contain at least 15 characters and are stored with
bcrypt.

Run the authorized local security smoke suite after starting the stack:

```powershell
.\security-smoke.ps1
```

The test refuses non-loopback targets unless `-AllowRemote` is explicitly
provided. See [`SECURITY.md`](SECURITY.md) for the security model, automated
scans, and responsible disclosure instructions.

## Deployment

The Render backend blueprint is defined in [`render.yaml`](render.yaml). Container definitions are maintained separately in [`backend/Dockerfile`](backend/Dockerfile) and [`frontend/Dockerfile`](frontend/Dockerfile).

GitHub Actions publishes two images to GitHub Container Registry:

- `ghcr.io/mustafa-oezdemir/banking_go-backend`
- `ghcr.io/mustafa-oezdemir/banking_go-frontend`

## Why This Project Exists

This repository is a practical fintech-backend demonstration covering:

- correctness under concurrency
- auditable money movement
- clear API boundaries
- production-minded deployment shape

If you are a recruiter or reviewer, start with this README and the Swagger UI. For the full technical narrative, read the FreeCodeCamp article linked above.
