# Workload identity and IAM (ADR-004)

ADR-004 selects **one ECS task role per process**, an environment-scoped ECS
execution role, and Secrets Manager-injected database credentials. Tasks carry
no static AWS access keys. RDS IAM database authentication and automated
rotation are deferred.

## Roles

| Role | Attached to | Permissions |
|---|---|---|
| `atlas-<env>-ecs-execution` | All ECS services/tasks in the environment | ECR pull, CloudWatch Logs write, `secretsmanager:GetSecretValue` on `atlas/<env>/*` only |
| `atlas-<env>-api` | `atlas-api` | None by default; add scoped policies only when the API calls an AWS service |
| `atlas-<env>-mcp` | `atlas-mcp` | None by default |
| `atlas-<env>-worker` | `atlas-worker` | None by default |
| `atlas-<env>-scheduler` | `atlas-scheduler` | None by default |
| `atlas-<env>-python` | Python extraction worker | None by default |
| `atlas-<env>-migrate` | one-off `atlas-migrate` task | None by default |
| `atlas-<env>-provision` | one-off `atlas-provision` task | None by default |
| `atlas-<env>-otel` | ADOT collector gateway | X-Ray segment/telemetry write; CloudWatch metrics log group only |

An empty task-role policy is deliberate: the applications do not call AWS APIs
today. Database, WorkOS and R2 credentials arrive as ECS-injected secrets, and
R2 uses external S3-compatible keys rather than AWS IAM. The one exception is
`atlas-<env>-otel`, which writes telemetry to X-Ray and the environment's
CloudWatch metrics log group.

Templates: `iam/ecs-task-trust-policy.json`, `iam/ecs-execution-policy.json`,
`iam/task-role-policy.json`, `iam/otel-role-policy.json`. Replace `<ACCOUNT_ID>`,
`<REGION>` and `<ENV>`.

## Database role mapping

| Process | `DATABASE_URL` role | `AUTH_DATABASE_URL` role |
|---|---|---|
| `atlas-api` | `atlas_app` | `atlas_auth` |
| `atlas-mcp` | `atlas_app` | n/a |
| `atlas-worker` | `atlas_worker` | n/a |
| `atlas-scheduler` | `atlas_worker` | n/a |
| Python worker | `atlas_worker` | n/a |
| `atlas-migrate` / `atlas-provision` | `atlas_migrator` | n/a |

The API and MCP may share `atlas_app` because they have the same tenant-data
capability; privilege classes are never shared. One-off tasks are the only
holders of the migrator URL.

## Secrets

| Secret | Consumers |
|---|---|
| `atlas/<env>/database/app` | api, mcp |
| `atlas/<env>/database/auth` | api |
| `atlas/<env>/database/worker` | worker, scheduler, python |
| `atlas/<env>/database/migrator` | migrate, provision |
| `atlas/<env>/workos` | api |
| `atlas/<env>/r2` | api, worker (document paths) |
| `atlas/<env>/secret-key` | api, worker |

Rotation target is 90 days for the sandbox (manual: rotate the RDS password,
update the secret version, redeploy consumers). Long-lived passwords are the
accepted cost of deferring RDS IAM auth.

## Guardrails

- No `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` in task definitions or CI
  environment variables. CI uses GitHub OIDC federation to a deploy role, not
  static keys.
- Task trust policies must include `aws:SourceAccount` (and preferably a
  `aws:SourceArn`/cluster condition where available).
- A task role must not be reusable across environments.
- Any new AWS call needs a scoped policy on the calling process's task role and
  a review note; do not broaden the execution role.
- Future service-to-service calls use IAM SigV4 or mTLS, never a shared bearer
  secret.

## Verification checklist

1. In each task, `aws sts get-caller-identity` (temporary debug image) shows the
   process-specific role ARN.
2. A task role cannot read another environment's secrets.
3. `atlas_app` cannot read `users`/`sessions`; `atlas_worker` cannot read
   identity tables; `atlas_migrator` is only used by one-off tasks.
4. Task definitions contain `secrets` references, not plaintext values.
5. Revoking a task role or deleting its secret blocks new tasks; running tasks
   pick up changes on redeploy.
