# ADR-011: Distributed Observability Stack

- Status: Accepted
- Date: 2026-09-11

## Decision

Services emit structured JSON logs with a `service` name and propagate `X-Request-ID` as the correlation identifier. Each Go HTTP edge uses OpenTelemetry W3C trace-context extraction and span creation; Compose exports OTLP/HTTP traces to Jaeger. The Gateway exposes Prometheus-compatible edge request and upstream-failure counters at `/metrics`; Banking, Outbox and RabbitMQ components already log committed publish/consumer failures without event payloads. RabbitMQ management provides broker, queue and dead-letter operational state in local Compose.

Jaeger is deliberately limited to local Docker learning use. The OTLP endpoint is optional outside Compose, so service binaries remain runnable without a collector. No log, span or metric may contain passwords, JWTs, reset tokens, full IBANs, account balances, or notification payloads.

## Signals

- HTTP request count/latency/errors: gateway structured log and `/metrics`.
- Payment booking failures: Banking payment failure audit and structured log.
- Outbox backlog: `outbox_events` where `published_at IS NULL`.
- Consumer failures/retries/DLQ: Notification/RabbitMQ structured logs and RabbitMQ management.
