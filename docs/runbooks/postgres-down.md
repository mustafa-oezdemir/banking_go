# PostgreSQL down

1. Stop accepting assumptions: Banking, Identity and Notification deduplication persistence require PostgreSQL.
2. Check PostgreSQL health, disk capacity and logs; restore database availability before restarting dependent services.
3. Verify migrations are at the expected version and inspect outbox backlog after recovery.
4. Do not manually mark financial events published or processed without reconciliation.
