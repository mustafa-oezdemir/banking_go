# Notification service down

1. Check `docker compose ps notification-service` and `GET /health` on port 8490.
2. Inspect structured logs by `request_id`; do not copy email payloads or reset tokens into tickets.
3. Payment events remain in RabbitMQ/outbox and retry according to Phase 4 semantics. Restore the service, then verify the notification queue and dead-letter queue drain.
4. Password reset delivery is best effort; request a new reset after recovery.
