# Outbox backlog

Use the Banking database:

```sql
SELECT count(*) FROM outbox_events WHERE published_at IS NULL;
```

Check the outbox runner, RabbitMQ health and publish error logs. A backlog during broker downtime is expected. Once the broker recovers it should decline without manual replay. Investigate rows with increasing `publish_attempts` or `last_error`; do not delete outbox rows to clear an alert.
