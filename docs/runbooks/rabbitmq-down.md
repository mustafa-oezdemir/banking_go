# RabbitMQ down

1. Confirm RabbitMQ health and management UI availability (default local port 15672).
2. Banking must remain able to commit payments: committed events remain unpublished in `outbox_events`.
3. Restore the broker. The outbox runner reconnects and republishes; duplicate delivery is expected and the Notification consumer deduplicates event IDs.
4. Inspect dead-letter queues before requeueing poison messages.
