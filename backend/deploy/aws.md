# AWS deployment shape (ADR-001)

ADR-001 selects **AWS** as the primary runtime for Atlas. This document is the
implementation runbook for the sandbox and the template for the Phase 10
sandbox pilot. It does not create infrastructure; Terraform/console resources
are provisioned per environment. ADR-010 fixes the sandbox/v0 region as
`eu-west-1`; customer-specific residency and dedicated tiers remain deferred.

## Process topology

| Process | AWS runtime | Ingress | Notes |
|---|---|---|---|
| `atlas-api` | ECS Fargate service | ALB HTTPS, host `api.<env>.<domain>` | Human REST + OIDC callback. Readiness needs migrations applied. |
| `atlas-mcp` | ECS Fargate service | ALB HTTPS, host `mcp.<env>.<domain>` | Streamable HTTP `POST /mcp`; ALB idle timeout ≥ 120s for SSE. Refuses `ATLAS_ENV=production` until ADR-002/ADR-004 token issuance. |
| `atlas-worker` | ECS Fargate service | None | Scale on queue depth or fixed replicas; at-least-once handlers. |
| `atlas-scheduler` | ECS Fargate service | None | One replica; outbox dispatch, lease reap, approval expiry. Extra replicas are lock-safe but unnecessary. |
| Python extraction worker | ECS Fargate service | None | Runs `python/atlas_worker.py`; writes only job results. |
| `atlas-otel` | ECS Fargate service | None | ADOT collector gateway; only service with X-Ray/CloudWatch write permissions. |
| `atlas-migrate` | One-off ECS task | None | `up` after every deploy that adds migrations. |
| `atlas-provision` | One-off ECS task | None | Re-run after migrations so new tables/functions get role grants. |

## Images

One Go image definition; pick the process at build time.

```bash
docker build --build-arg SERVICE=atlas-api       -t atlas-api       backend/
docker build --build-arg SERVICE=atlas-mcp       -t atlas-mcp       backend/
docker build --build-arg SERVICE=atlas-worker    -t atlas-worker    backend/
docker build --build-arg SERVICE=atlas-scheduler -t atlas-scheduler backend/
docker build --build-arg SERVICE=atlas-migrate   -t atlas-migrate   backend/
docker build --build-arg SERVICE=atlas-provision -t atlas-provision backend/
docker build -f backend/Dockerfile.python -t atlas-python-worker backend/
```

Push to ECR per service. The runtime image is distroless, runs as nonroot and
includes `/app/migrations` and `/app/scripts` plus `MIGRATIONS_DIR`.

## Network

- VPC with two private subnets (tasks + RDS) and two public subnets (ALB).
- NAT gateway for egress to WorkOS, Cloudflare R2 and ECR/CloudWatch endpoints.
- RDS accepts connections only from the task security group; no public access.
- ALB terminates TLS (ACM), forwards API/MCP by host or path.

## Data and secrets

- **RDS for PostgreSQL 16**, encrypted, automated backups with PITR. Create the
  runtime roles with `scripts/provision-app-role.sql` using the migrator
  credentials; do not run the API as the owner.
- **Cloudflare R2** remains the object store (`STORAGE_DRIVER=s3`). Store the R2
  access keys in Secrets Manager; they are not AWS IAM credentials.
- **Secrets Manager** entries: `atlas/database` (migrator/app/auth/worker URLs),
  `atlas/workos` (client ID/API key/redirect/authkit domain), `atlas/r2`
  (endpoint/bucket/keys), `atlas/secret-key` (`ATLAS_SECRET_KEY`).
- ECS execution role may read only those secrets and write logs. Each service
  gets a distinct IAM task role (ADR-004) with no AWS data-plane permissions by
  default. See [`iam.md`](iam.md) for the role matrix, database-role mapping,
  rotation and verification checklist.

## Workload identity (ADR-004)

Create the shared execution role and one task role per process using
`iam/ecs-task-trust-policy.json`, `iam/ecs-execution-policy.json` and
`iam/task-role-policy.json`. Never place static AWS keys in task definitions.
Database URLs map to the four existing roles: `atlas_api` uses `atlas_app` +
`atlas_auth`, MCP uses `atlas_app`, workers/scheduler/Python use `atlas_worker`,
and one-off migration/provisioning tasks use `atlas_migrator`. Full matrix:
[`iam.md`](iam.md).

## Environment per service

`atlas-api`:

```text
ATLAS_ENV=staging
PORT=8080
DATABASE_URL=<atlas_app>
AUTH_DATABASE_URL=<atlas_auth>
WORKOS_CLIENT_ID / WORKOS_API_KEY / WORKOS_REDIRECT_URI / WORKOS_AUTHKIT_DOMAIN
STORAGE_DRIVER=s3
R2_ENDPOINT / R2_REGION / R2_BUCKET / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY
ATLAS_SECRET_KEY=<32-byte base64/hex>
LOG_LEVEL=info
```

`atlas-mcp`:

```text
ATLAS_ENV=staging
MCP_PORT=8081
DATABASE_URL=<atlas_app>
LOG_LEVEL=info
```

`atlas-worker` / scheduler / Python worker:

```text
WORKER_DATABASE_URL=<atlas_worker>
WORKER_ID=<task-id or hostname>
ATLAS_SECRET_KEY=<same key as the API>
JOB_LEASE_SECONDS / JOB_POLL_MS / OUTBOX_POLL_MS / LEASE_REAP_MS / APPROVAL_EXPIRY_MS
```

`atlas-otel`:

```text
ATLAS_ENV=staging
AWS_REGION=eu-west-1
```

All application services also receive (once OTel instrumentation lands):

```text
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.atlas-<env>.local:4317
OTEL_SERVICE_NAME=<process workload name>
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=<env>,service.namespace=atlas
```

## Deploy order

1. Apply migrations: one-off `atlas-migrate up`.
2. Re-run `atlas-provision` so new tables/functions receive grants.
3. Deploy/refresh `atlas-worker`, `atlas-scheduler`, Python worker.
4. Deploy `atlas-api` and `atlas-mcp`; wait for ALB health checks.
5. Verify `GET /healthz` and `GET /readyz` on the API and `GET /healthz` on MCP.

Rollbacks use the previous ECR image tag. Migrations are forward-only; a
rollback that requires a schema reversal must run the matching down migration
as a deliberate, reviewed one-off task.

## Health checks

- API: ALB target `GET /readyz` (checks database and pending migrations).
- MCP: ALB target `GET /healthz` (unauthenticated, no database call).
- Workers/scheduler: ECS container health is process liveness; queue health is
  monitored through backlog metrics (ADR-006).

## Observability

ADR-006 selects CloudWatch + X-Ray behind an ADOT collector gateway. See
[`observability.md`](observability.md) for the topology, collector config,
log-based metrics, alarms, dashboards and retention. Application task roles
remain free of AWS write permissions; only `atlas-otel` can write telemetry.

## Sandbox pilot notes

Phase 10 runs against this AWS sandbox with realistic synthetic data rather
than a live customer. Keep `ATLAS_ENV=staging`; do not enable the production
OIDC gate or claim production readiness until ADR-002/ADR-004/ADR-010 are
finalized. The R2 bucket and WorkOS environment must be sandbox-scoped.
