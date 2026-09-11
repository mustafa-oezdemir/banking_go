
# TASK: Add production-style RabbitMQ monitoring with Prometheus + Grafana



Project:
Pehlione DemoBank / banking_go

IMPORTANT:
First inspect the existing repository and current Docker Compose architecture.

Do NOT blindly add infrastructure.

RabbitMQ must already exist in the architecture before implementing this task.
If RabbitMQ has not yet been introduced, STOP and report:

"RabbitMQ monitoring should be implemented after the RabbitMQ + Outbox phase."

Do not introduce RabbitMQ merely for the purpose of monitoring.

Reference documentation:
https://www.rabbitmq.com/docs/prometheus

Primary goal:
When the local Docker environment starts, RabbitMQ monitoring must work automatically with:

RabbitMQ
    ↓ metrics
Prometheus
    ↓ datasource
Grafana
    ↓
RabbitMQ Overview Dashboard

There must be no manual Grafana configuration required after docker compose up.

==================================================

1. INSPECT CURRENT STATE
   ==================================================

Before changing anything inspect:

- docker-compose.yml / compose files
- RabbitMQ image/version
- RabbitMQ configuration
- enabled plugins
- existing observability folders
- existing Prometheus configuration
- existing Grafana configuration
- RabbitMQ exchanges
- queues
- vhosts
- DLQs
- notification consumer
- outbox publisher
- Docker networks
- exposed ports
- environment variables
- health checks

Do not assume names or paths.

Adapt the implementation to the repository.

==================================================
2. RABBITMQ PROMETHEUS PLUGIN
=============================

Use RabbitMQ's built-in:

rabbitmq_prometheus

Do NOT introduce an unnecessary third-party RabbitMQ exporter.

Ensure the plugin is enabled declaratively and reproducibly.

Prefer an enabled_plugins file or the project's existing RabbitMQ configuration mechanism.

RabbitMQ should expose Prometheus metrics through:

port: 15692
path: /metrics

The monitoring endpoint should primarily be available to the internal Docker network.

Do not expose infrastructure ports publicly unless there is a concrete development requirement.

For local development, exposing localhost ports is acceptable when useful.

Verify:

curl http://localhost:15692/metrics

or the equivalent request from inside the Docker network.

It must return Prometheus-formatted RabbitMQ metrics.

==================================================
3. RABBITMQ METRIC COLLECTION INTERVAL
======================================

Follow the RabbitMQ monitoring documentation.

Configure an appropriate RabbitMQ statistics collection interval.

Use:

collect_statistics_interval = 10000

unless the repository already has a justified different configuration.

Explain why:

RabbitMQ does not need to regenerate statistics more frequently than Prometheus consumes them.

Do not tune this blindly if an existing requirement depends on another interval.

==================================================
4. PROMETHEUS
=============

Add Prometheus as a Docker Compose service if it does not already exist.

Create repository-managed configuration, for example:

observability/
  prometheus/
    prometheus.yml

or adapt to the repository's existing structure.

Configure Prometheus to scrape RabbitMQ.

Recommended initial scrape interval:

15s

Example architecture:

Prometheus
  -> rabbitmq:15692/metrics

Use Docker service discovery / service hostname.

Do NOT configure Prometheus to scrape localhost from inside the container.

Example logical target:

rabbitmq:15692

not:

localhost:15692

Verify the RabbitMQ target appears as UP in Prometheus.

==================================================
5. AGGREGATED METRICS BY DEFAULT
================================

Keep RabbitMQ's normal aggregated:

/metrics

endpoint as the default metrics source.

Do NOT globally enable:

prometheus.return_per_object_metrics = true

without a demonstrated need.

Reason:

Per-object metrics can become expensive as queues, channels and connections grow.

Aggregated metrics should remain the default operational view.

==================================================
6. PROJECT-SPECIFIC QUEUE METRICS
=================================

We also want useful metrics for the DemoBank RabbitMQ queues.

First inspect the real queue/vhost names.

Do NOT invent names.

If queue-specific metrics are useful, add a SECOND Prometheus scrape configuration using:

/metrics/detailed

Prefer only the required metric families such as:

queue_coarse_metrics
queue_consumer_count

and, only if actually useful:

queue_delivery_metrics

Use vhost and/or queue filtering where appropriate.

Do NOT scrape every possible per-object RabbitMQ metric.

We want enough data to answer questions such as:

- Is the notification queue growing?
- Are consumers connected?
- How many messages are ready?
- How many messages are unacknowledged?
- Are consumers keeping up?
- Are messages being redelivered?
- Is a DLQ accumulating messages?

Keep monitoring cardinality controlled.

==================================================
7. GRAFANA
==========

Add Grafana as a Docker Compose service if it does not already exist.

Provision Grafana automatically.

There must be NO requirement for the developer to manually:

- create the Prometheus datasource
- import dashboards
- enter a dashboard ID after startup

Use Grafana provisioning files.

Suggested structure, adapted as needed:

observability/
  grafana/
    provisioning/
      datasources/
        prometheus.yml
      dashboards/
        dashboards.yml
    dashboards/
      rabbitmq-overview.json

Prometheus should automatically become the Grafana datasource.

Use a stable datasource UID if dashboard provisioning requires one.

==================================================
8. OFFICIAL RABBITMQ DASHBOARD
==============================

Use the official RabbitMQ-maintained / recommended RabbitMQ Overview dashboard as the baseline.

Reference:

https://grafana.com/grafana/dashboards/10991-rabbitmq-overview/

Prefer storing the dashboard JSON in the repository so that:

docker compose up

produces a working dashboard without requiring runtime internet access.

Do not create an inferior dashboard from scratch if the official RabbitMQ dashboard already covers the requirement.

Verify compatibility with the RabbitMQ and Grafana versions actually used by the repository.

If the official JSON requires a small compatibility adjustment, document the change instead of silently replacing the dashboard.

==================================================
9. DEMOBANK RABBITMQ DASHBOARD
==============================

In addition to the general RabbitMQ Overview dashboard, create a SMALL project-specific dashboard only if the actual RabbitMQ topology justifies it.

Name:

DemoBank - RabbitMQ Messaging

This dashboard should focus on application-level messaging health.

Potential panels, depending on available real metrics:

RabbitMQ Health

- broker target UP
- connections
- channels
- consumers

Queue Health

- ready messages
- unacknowledged messages
- queue depth
- consumer count

Message Flow

- publish rate
- delivery rate
- acknowledgement rate
- redelivery rate

Reliability

- unroutable messages
- redelivered messages
- DLQ depth if a DLQ exists

Resource Safety

- memory usage / memory alarm proximity
- disk available / disk alarm proximity
- file descriptor usage

DO NOT invent metrics.

Inspect the real RabbitMQ Prometheus metric names produced by the running version.

Build PromQL against actual metrics.

==================================================
10. DASHBOARD THRESHOLDS
========================

Do not pretend there is one universally correct threshold.

RabbitMQ documentation explicitly notes that thresholds depend on workload.

Therefore:

Use official dashboard defaults where appropriate.

For DemoBank-specific panels, avoid arbitrary production SLO numbers unless they already exist as project requirements.

Safe examples:

Consumer count == 0 while queue contains messages
    -> unhealthy signal

RabbitMQ Prometheus target DOWN
    -> unhealthy signal

DLQ contains messages
    -> warning / investigation signal

Queue continuously growing
    -> operational warning candidate

Do not invent statements such as:

"queue depth > 100 is always critical"

unless the project has defined that requirement.

==================================================
11. ALERTING
============

This task is primarily dashboard + metrics infrastructure.

However prepare the monitoring structure so future Prometheus alert rules can be added cleanly.

If adding a minimal rule set is straightforward, only add high-signal alerts such as:

- RabbitMQ metrics target unavailable
- no consumer while messages are waiting
- DLQ contains messages

Avoid creating a large noisy alert rule set.

If alert thresholds are not justified yet, document them as:

To Be Validated

rather than guessing numbers.

==================================================
12. DOCKER COMPOSE
==================

The relevant development architecture should eventually resemble:

DemoBank services
        |
        v
     RabbitMQ
        |
        | :15692 /metrics
        v
    Prometheus
        |
        v
      Grafana

Add:

Prometheus persistent volume

Grafana persistent volume if useful

Grafana provisioning mounts

Prometheus configuration mount

RabbitMQ configuration/plugin mounts

Use health checks where they provide real value.

Do not create artificial depends_on chains that imply application correctness.

==================================================
13. SECURITY
============

This is currently a learning/local development environment.

Nevertheless follow sane boundaries:

- do not put passwords in Git
- do not expose monitoring ports unnecessarily
- do not use production secrets as defaults
- monitoring endpoints should preferably remain inside the Docker network
- Grafana credentials must be configurable with environment variables
- document development defaults
- production deployments must not use default admin credentials

RabbitMQ supports authentication/TLS for its Prometheus endpoint.

Do not add unnecessary TLS complexity to the local Docker environment.

Document it as a production hardening option instead.

==================================================
14. DOCUMENTATION
=================

Add/update documentation explaining:

NEDİR?
What Prometheus and Grafana do.

NEDEN?
Why RabbitMQ must be observable in an event-driven architecture.

NASIL?
How DemoBank collects and visualises RabbitMQ metrics.

Document local URLs, based on the actual Compose configuration:

RabbitMQ Management UI
Prometheus
Grafana

Document how to verify:

RabbitMQ metrics endpoint

Prometheus target

Grafana datasource

RabbitMQ Overview dashboard

DemoBank RabbitMQ dashboard, if created

Also document troubleshooting:

RabbitMQ target DOWN
dashboard shows No Data
Prometheus cannot resolve rabbitmq
consumer count is zero
queue depth continuously increases

==================================================
15. ARCHITECTURE DECISION
=========================

Create an ADR if this is the first observability infrastructure introduced into the architecture.

Example:

ADR-XXX: Monitor RabbitMQ with Prometheus and Grafana

Use the project ADR format:

ADR-XXX: [Decision]

Status:
Context:
Problem:
Decision Drivers:
Options Considered:
Selected Decision:
Positive Consequences:
Negative Consequences:
Risks:
Mitigations:
Advice / Stakeholder Input:
Revisit Conditions:

Decision should capture:

Selected:
RabbitMQ built-in Prometheus plugin
+
Prometheus
+
Grafana
+
official RabbitMQ dashboard

Explicitly reject adding an unnecessary third-party RabbitMQ exporter.

Trade-offs must include:

GAIN:

- operational visibility
- queue/backlog visibility
- broker resource visibility
- easier debugging
- observable async architecture

LOSE:

- additional containers
- more memory/CPU usage
- monitoring configuration maintenance
- dashboard/version compatibility maintenance

==================================================
16. VALIDATION
==============

Before considering this task finished, actually run the stack.

Validate at minimum:

docker compose config

docker compose up -d

docker compose ps

RabbitMQ healthy/running

RabbitMQ Prometheus endpoint responds

Prometheus starts successfully

RabbitMQ Prometheus target == UP

Grafana starts successfully

Grafana datasource is automatically provisioned

RabbitMQ Overview dashboard exists automatically

Dashboard has real data

No repeated container restart loops

No obvious errors in:

docker compose logs rabbitmq
docker compose logs prometheus
docker compose logs grafana

If DemoBank RabbitMQ queues already exist:

generate or use real application traffic and verify that queue/message metrics visibly change.

==================================================
17. DO NOT BREAK EXISTING FUNCTIONALITY
=======================================

Monitoring is an infrastructure concern.

It must NOT modify:

financial booking logic
ledger invariants
payment state machine
idempotency behaviour
outbox transaction semantics
consumer acknowledgement semantics

Do not refactor unrelated application code.

==================================================
18. GIT RULE
============

This task must be isolated as its own architecture/infrastructure change.

Before modifying files:

git status
git branch --show-current
git remote -v

If pre-existing uncommitted user changes exist:
STOP and report them.

After implementation:

run validations
review git diff
ensure no secrets are committed
update documentation
update ADR
commit all intentional changes

Suggested commit:

observability: add RabbitMQ Prometheus and Grafana monitoring

Then:

git push

Push the current feature branch.

NEVER force push.

If push fails:
STOP and report the failure.

Do not continue pretending the phase succeeded.

==================================================
19. FINAL REPORT
================

At completion report:

STATUS

ARCHITECTURE CHANGE

FILES CHANGED

RABBITMQ CONFIGURATION

PROMETHEUS CONFIGURATION

GRAFANA CONFIGURATION

DASHBOARDS

METRICS VERIFIED

TEST / VALIDATION COMMANDS

RESULTS

ADR

COMMIT HASH

BRANCH

PUSH RESULT

TRADE-OFFS

WHAT I SHOULD LEARN

Also explain:

NEDİR?
Prometheus scraping and Grafana dashboards.

NEDEN?
Why RabbitMQ observability matters for asynchronous systems.

NASIL?
How metrics travel from RabbitMQ -> Prometheus -> Grafana.

Do not mark the task complete unless the dashboard actually contains real RabbitMQ data.
