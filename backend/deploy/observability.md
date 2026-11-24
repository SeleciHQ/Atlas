# Observability backend (ADR-006)

ADR-006 selects **Amazon CloudWatch + AWS X-Ray**, fed by a single
**ADOT/OpenTelemetry collector gateway**, for the sandbox and v0. Application
code stays vendor-neutral: it emits OpenTelemetry over OTLP and structured JSON
logs; the collector owns all AWS exports. Swapping to another backend later is
a collector change, not an application change.

## Topology

| Signal | Path | Retention (sandbox) |
|---|---|---|
| Structured logs | container stdout → CloudWatch Logs `/atlas/<env>/<service>` | 30 days |
| Traces | app OTLP → collector → X-Ray | 30 days |
| Metrics | app OTLP → collector → CloudWatch EMF (`/atlas/<env>/metrics`) | 15 months (CloudWatch default) |

The collector runs as its own ECS service (`atlas-otel`) with task role
`atlas-<env>-otel`; application task roles stay free of AWS data-plane
permissions (ADR-004). Service discovery: Cloud Map
`otel-collector.atlas-<env>.local:4317` (gRPC) / `:4318` (HTTP). One replica is
enough for the sandbox; two replicas require DNS round-robin or an internal NLB.

Collector config: [`otel/collector.yaml`](otel/collector.yaml). IAM policy:
[`iam/otel-role-policy.json`](iam/otel-role-policy.json).

## Application configuration

Set on every process once OpenTelemetry SDK instrumentation is enabled:

```text
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.atlas-<env>.local:4317
OTEL_SERVICE_NAME=atlas-api|atlas-mcp|atlas-worker|atlas-scheduler|atlas-python-worker
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=<env>,service.namespace=atlas
```

Current state: processes emit structured JSON logs; HTTP access logs include
method, path, status, duration and request id. Queue/job logs include `job_failed`,
`claim_failed`, `leases_reaped`, `approvals_expired` and
`operation_fail_record_failed`. OTLP trace/metric export is the remaining
instrumentation task (Phase 10); the collector pipeline is ready for it.

## Log-based metrics (available now)

CloudWatch Logs metric filters over the JSON log groups:

| Metric | Filter |
|---|---|
| `Atlas/Http5xx` | `{ $.msg = "http_request" && $.status >= 500 }` |
| `Atlas/Http403` | `{ $.msg = "http_request" && $.status = 403 }` |
| `Atlas/JobFailed` | `{ $.msg = "job_failed" }` |
| `Atlas/JobCompleteFailed` | `{ $.msg = "job_complete_failed" }` |
| `Atlas/LeaseReapFailed` | `{ $.msg = "lease_reap_failed" }` |
| `Atlas/ApprovalExpiryFailed` | `{ $.msg = "approval_expiry_failed" }` |
| `Atlas/OperationFailRecordFailed` | `{ $.msg = "operation_fail_record_failed" }` |

Once OTLP metrics are instrumented, add gauges for queue depth by status,
oldest pending age, dead-letter count, operation failures, reconciliation
throughput and storage checksum failures.

## Alarms (sandbox)

1. ALB target 5xx > 1% for 5 minutes (API and MCP).
2. ECS service running tasks below desired for 5 minutes (all services).
3. ECS CPU or memory > 85% for 10 minutes.
4. RDS CPU > 80%, free storage < 20%, connections > 80% of max.
5. `JobFailed` or `JobCompleteFailed` > 0 in 10 minutes.
6. `LeaseReapFailed` or `ApprovalExpiryFailed` > 0 in 10 minutes.
7. `OperationFailRecordFailed` > 0 in 10 minutes.
8. Dead-letter jobs > 0 (OTLP metric when available; interim log alarm).
9. Collector task unhealthy / OTLP export errors.

## Dashboards

One `Atlas / <env>` dashboard: ALB requests/5xx/latency, per-service ECS
CPU/memory/restarts, RDS health, job throughput/failures/dead letters, queue
age, reconciliation runs by status, approval expiry, operation failures,
authorization denials (403s), storage checksum failures.

## Security and retention

- No document contents, tokens, credentials or raw payloads in spans or logs;
  reference records by ID.
- The collector role can only write X-Ray segments and the environment's metrics
  log group. Application task roles have no AWS data-plane permissions.
- Sandbox retention is 30 days for logs/traces; CloudWatch metrics default 15
  months. ADR-007 owns retention policy for business data, not telemetry.
- Logs are accessed through CloudWatch; no public dashboards.

## Migration path

Replace the `awsxray`/`awsemf` exporters with an OTLP exporter to the selected
vendor, or add a second pipeline. No application or SDK change is required.
Revisit this choice if trace volume, query needs or cost exceed CloudWatch/X-Ray
at pilot scale.
