# Build application binary and migration tool
FROM golang:1.26.5-alpine AS builder
WORKDIR /src

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/. .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ledger ./cmd/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/payment-worker ./cmd/worker
RUN GOBIN=/out go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

# Minimal runtime image
FROM alpine:3.21
WORKDIR /app

COPY --from=builder /out/ledger /usr/local/bin/ledger
COPY --from=builder /out/payment-worker /usr/local/bin/payment-worker
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /src/docs ./docs
COPY --from=builder /src/postgres/migrations ./postgres/migrations
COPY backend/docker-entrypoint /usr/local/bin/entrypoint

ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt \
	TZ=UTC

RUN chmod +x /usr/local/bin/entrypoint && \
	sed -i 's/\r$//' /usr/local/bin/entrypoint && \
	adduser -D -u 10001 appuser && \
	chown -R appuser:appuser /app

USER appuser

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
	CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/bin/sh", "/usr/local/bin/entrypoint"]
