# ADR-013: RabbitMQ monitoring with Prometheus and Grafana

Status: Accepted

Context: Payment notification delivery uses a transactional outbox, RabbitMQ retry routing, and a dead-letter queue. The broker can be healthy while the queue flow is not, so container health alone is not sufficient operational evidence.

Problem: Add low-cardinality, production-style RabbitMQ visibility to the local Compose topology without exposing a new broker endpoint publicly or collecting detailed metrics for every dynamic queue.

Decision Drivers:

- make broker reachability, queue depth, consumer presence, and DLQ backlog observable;
- reuse RabbitMQ's supported Prometheus plugin and official Overview dashboard;
- keep collection cost bounded and the local stack easy to run;
- avoid credentials and sensitive payment data in metrics or dashboards.

Options Considered:

1. Use only the RabbitMQ Management UI and container health checks.
2. Add a bespoke metrics exporter or scrape Management API endpoints.
3. Enable RabbitMQ's built-in `rabbitmq_prometheus` plugin, scrape it with Prometheus, and provision Grafana dashboards.

Selected Decision: Option 3. RabbitMQ exposes its built-in metrics endpoint on the Compose-internal port `15692` with a 10-second statistics collection interval. Prometheus scrapes the aggregate `/metrics` endpoint every 15 seconds and scrapes `/metrics/detailed` only for `notification.payment.v1`, `notification.payment.retry`, and `notification.payment.dlq`. Grafana provisions the Prometheus datasource, the vendored official RabbitMQ Overview dashboard, and a small DemoBank queue-flow dashboard.

Positive Consequences:

- queue depth, DLQ depth, and consumer count are visible without Management UI polling;
- aggregate metrics avoid unbounded per-object cardinality;
- the focused dashboard matches the actual payment-notification queues and alert rules;
- monitoring configuration is versioned and recreated with Docker Compose.

Negative Consequences:

- Prometheus and Grafana add local containers, volumes, and resource usage;
- detailed queue metric names are tied to RabbitMQ's Prometheus plugin contract;
- alert rules are recorded locally but require an Alertmanager integration for external notification.

Risks:

- a broad detailed scrape can cause cardinality growth;
- dashboards can drift after a RabbitMQ plugin metric change;
- a weak Grafana password would expose operational data on a shared development host.

Mitigations:

- queue filtering is explicit in `observability/prometheus/prometheus.yml`;
- `/metrics` remains the aggregate default endpoint and the RabbitMQ Overview dashboard is retained as an upstream vendor artifact;
- Grafana requires its own generated `.env` password, disables anonymous access and signup, and the 15692 broker port has no host mapping;
- validation checks Prometheus targets and focused metric series after Compose startup.

Revisit Conditions: Add Alertmanager, long-term retention, remote storage, authentication proxying, or managed monitoring only when the deployment environment and an actionable operations process justify them.
