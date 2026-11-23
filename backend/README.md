# Atlas Backend (Go)

The Atlas backend is a Go modular monolith with independently runnable processes. The Next.js application
stays at the repository root; this directory contains only the production backend. See
[`../architecture.md`](../architecture.md) for the architecture and [`../roadmap.md`](../roadmap.md) for the
phase plan.

**Current status: Phase 2 + Phase 2.1 complete (2026-11-21); Phase 3 in progress.** Phase 1 is complete. The
Phase 2 security review verified the real provisioning SQL, separated identity/session database access from
tenant runtime access, and reduced human roles to Admin and Member. Phase 2.1 adds the WorkOS AuthKit adapter
(server-side authorization-code exchange), `oidc_identities` and organization invitations with the
invitation/linking rules, and OIDC session issuance. The real WorkOS sandbox E2E passed on 2026-11-21 (hosted
sign-in → JIT user → admin membership → session), so ADR-002 is Accepted and the local
`OIDC_PRODUCTION_VERIFIED=true` gate is set; production deployment still needs production WorkOS credentials.
Phase 3 has delivered migration `0005_jobs_outbox`, the `internal/jobs` queue/outbox package, the
worker/scheduler entrypoints, and the `0006_documents` upload/download lifecycle with local and R2 adapters
(`internal/storage`). A Python worker skeleton shows the result-only contract. Phase 3 is complete. Phase 4
(in progress) adds the connector contract, registry, mock/document-inbox adapters, envelope-encrypted
credentials, sync runs/cursors, and immutable source → normalized records with provenance. There are still
**no business handlers**. Phase 5 is complete (canonical v0 schema, ontology v1, read models, fixture corpus). Phase 6 (in progress) adds the extraction result contract, mock provider, validation gate, extraction drafts, and validated-draft promotion, the Python `document.extract` pipeline (scoped fetch + checksum), the Go `document.validate` gate, evidence-backed canonical promotion with VAT/email/exact-name candidate strength and ambiguity rejection, and a configurable HTTP extraction provider.

## Toolchain

| Tool | Version | Notes |
|---|---|---|
| Go | 1.27.4 or newer | `go.mod` pins `go 1.27.4`. May be installed per user (e.g. a local toolchain directory); it must be on `PATH`. |
| sqlc | 1.30.0 | Only needed when regenerating queries. |
| PostgreSQL | 16.x | Local development and CI must use the same major version. |
| bash or PowerShell | any supported version | `scripts/check.sh` / `scripts/check.ps1`. |

The development scripts never reference machine-specific installation paths; they resolve `go`, `gofmt` and
other tools from `PATH` and fail with a clear message when a required tool is missing. CI uses
`actions/setup-go` with `go-version-file: backend/go.mod`.

Install `sqlc` into the Go binary directory:

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
```

## Layout

```
cmd/atlas-api        REST API process
cmd/atlas-migrate    migration CLI (up, down, status)
cmd/atlas-provision  executes the role-provisioning SQL with passwords from the environment
cmd/atlas-mcp        MCP server stub (Phase 9)
cmd/atlas-worker     claims and executes jobs from the queue
cmd/atlas-scheduler  dispatches outbox events and reaps expired leases
internal/platform    config, logging, app errors, db pool, migrations, version
internal/identity    principals, roles, sessions, dev auth interfaces
internal/jobs        durable queue, leases, retries, dead-letter, outbox
internal/authorization RBAC permission catalogue and service
internal/orgs        organization-scoped read services
internal/tenancy     organization context and RLS-bound transactions
internal/audit       append-only audit attribution writer
internal/httpapi     router, middleware, auth, health/readiness, problem responses
internal/integration integration tests (embedded PostgreSQL or TEST_DATABASE_URL)
internal/testutil    disposable PostgreSQL for tests
migrations           versioned .up.sql/.down.sql files
scripts              portable developer verification scripts, role provisioning
```

Dependency rules (enforced by `internal/architecture_test.go`): `internal/` must not import `cmd/`; only
transport packages (`internal/httpapi`, `internal/mcpapi`) may be imported by transport code.

## Environment

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `DATABASE_URL` | yes | — | PostgreSQL URL (`postgres://` or `postgresql://`) |
| `ATLAS_ENV` | no | `development` | `development`, `test`, `staging`, `production` |
| `PORT` | no | `8080` | HTTP port for `atlas-api` |
| `DB_MAX_CONNS` | no | `10` | Connection pool maximum |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |
| `MIGRATIONS_DIR` | no | `migrations` | Migration directory (run from this directory) |
| `SHUTDOWN_TIMEOUT` | no | `10s` | Graceful shutdown deadline |
| `SERVICE_NAME` | no | `atlas-api` | Attributed in logs and health responses |
| `AUTH_DATABASE_URL` | no | `DATABASE_URL` | Dedicated identity/session role URL (`atlas_auth`); required for separated privileges outside development |
| `DEV_AUTH_ENABLED` | no | true in development/test, false otherwise | Development session login; rejected in staging/production |
| `SESSION_TTL` | no | `24h` | Development session lifetime (1h–720h) |
| `WORKOS_CLIENT_ID` | with OIDC | — | AuthKit client id; all three WorkOS values are required together |
| `WORKOS_API_KEY` | with OIDC | — | AuthKit secret key used for the server-side code exchange (never logged) |
| `WORKOS_REDIRECT_URI` | with OIDC | — | Must match the AuthKit redirect URI, e.g. `http://localhost:8080/v1/auth/oidc/callback` |
| `WORKOS_AUTHKIT_DOMAIN` | with OIDC | — | Hosted AuthKit domain (e.g. `https://team-staging.authkit.app`); sign-in starts at its root |
| `WORKOS_AUTHORIZE_URL` | no | AuthKit domain root | Explicit authorize-URL override (tests/mock providers); takes precedence over the domain |
| `WORKOS_AUTHENTICATE_URL` | no | `https://api.workos.com/user_management/authenticate` | Code-exchange endpoint override |
| `OIDC_PRODUCTION_VERIFIED` | no | `false` | Set `true` after the sandbox E2E passes (set for staging on 2026-11-21); production serving is refused until then |
| `ATLAS_ENV_FILE` | no | `.env.workos` | Dotenv file loaded at startup (existing environment wins) |
| `TEST_DATABASE_URL` | no | — | Dedicated test database; when unset, tests start embedded PostgreSQL |

## Commands

Run from `backend/`:

```bash
go run ./cmd/atlas-migrate status
go run ./cmd/atlas-migrate up
go run ./cmd/atlas-api
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

For a dedicated local database, set `DATABASE_URL` before running the commands. `atlas-api` keeps liveness
and readiness separate: `/healthz` is process liveness; `/readyz` requires database connectivity **and** no
pending migrations.

## Identity, roles and development authentication

Atlas distinguishes four principal categories: **human**, **external agent**, **integration** and **Atlas
workload**. Phase 2 implements human principals and models the other three (tables and Go types); agent and
integration authentication are deferred to Phase 9. Workload identity defines an interface only (ADR-004).

Human memberships hold exactly one of two built-in roles. Departmental job titles are not built-in roles;
permissions are the authorization mechanism:

| Role | Capabilities (Phase 2 catalogue) |
|---|---|
| `admin` | organization read/manage, members read/manage, agents manage |
| `member` | organization read, members read; eligible members may later be granted `actions.approve` / `actions.reject` |

Permissions are enforced centrally by `internal/authorization`; handlers never implement their own rules.
Unknown roles and non-human principals are denied by default. `actions.approve` and `actions.reject` are
defined as explicit permissions but are **not** granted by role in Phase 2; they will be granted to eligible
members when approval workflows exist (Phase 8). There is intentionally no human `actions.execute` permission:
execution remains an Atlas-controlled process.

### Development-only login

Production human authentication remains managed OIDC (ADR-002, still open). For development and tests, Atlas
uses opaque, database-backed sessions behind the `identity.Authenticator` interface. The dev login route is
registered only when `DEV_AUTH_ENABLED` is true and is refused in staging/production by configuration.
`atlas-api` also refuses to serve in `production` until managed OIDC is implemented (`Config.ValidateServing`),
so a production deployment cannot accidentally expose the development path.

Sessions are 256-bit opaque tokens; only the SHA-256 hash is stored. Expiry (`SESSION_TTL`) and revocation
(logout deletes the session) are enforced on every request and covered by integration tests.

Seed an organization and member, then log in. Identity/session operations use `AUTH_DATABASE_URL` (the
`atlas_auth` role); tenant endpoints use `DATABASE_URL` (the `atlas_app` role). In development both may point
at the same privileged URL before roles are provisioned.

```sql
-- Run as a privileged provisioning role.
INSERT INTO organizations (name) VALUES ('Acme Logistics') RETURNING id;
INSERT INTO users (email, display_name) VALUES ('amina@acme.test', 'Amina Diallo') RETURNING id;
INSERT INTO memberships (organization_id, user_id, role)
VALUES ('<org-id>', '<user-id>', 'admin');
```

```bash
curl -c cookies.txt -X POST http://localhost:8080/v1/auth/dev-login \
  -H "Content-Type: application/json" \
  -d '{"organization_id":"<org-id>","email":"amina@acme.test"}'

curl -b cookies.txt http://localhost:8080/v1/auth/me
curl -b cookies.txt http://localhost:8080/v1/members
curl -b cookies.txt http://localhost:8080/v1/organization/settings   # admin only
curl -b cookies.txt -X POST http://localhost:8080/v1/auth/logout
```

### Managed OIDC (implemented; sandbox E2E passed 2026-11-21)

Phase 2.1 implements the WorkOS AuthKit adapter behind `identity.Authenticator`:

- `GET /v1/auth/oidc/start` — builds the hosted AuthKit authorization URL (the environment's
  `WORKOS_AUTHKIT_DOMAIN` root with `client_id`, `redirect_uri`, `response_type`, `state`), sets a short-lived
  sealed state cookie (HMAC), and accepts optional `organization_id`, `login_hint`, or `link=1` (explicit
  linking; requires an authenticated session). `WORKOS_AUTHORIZE_URL` overrides the base for tests/mocks.
- `GET /v1/auth/oidc/callback` — validates state, performs the server-side authorization-code exchange with
  WorkOS, resolves/creates the Atlas user, accepts a pending invitation when allowed, and issues the Atlas
  session cookie. `format=json` returns the principal instead of redirecting (useful for tests).
- `POST /v1/invitations` — admin-only (`members:manage`) invitation creation; returns the one-time accept token
  for a share link. Email delivery is not implemented yet.

Identity rules: the external identity key is the unique `(issuer, subject)` pair; new identities JIT-create the
user record; organization membership is granted only by accepting a pending invitation after the IdP asserts
`email_verified`, the invitation is unexpired/unaccepted, and the email matches. An existing Atlas email without
an identity mapping returns `account_linking_required` (never auto-linked); linking an existing account requires
an authenticated session plus a fresh OIDC authentication. No silent identity merges.

Production gate: `atlas-api` refuses `ATLAS_ENV=production` unless WorkOS configuration is present **and**
`OIDC_PRODUCTION_VERIFIED=true`; development authentication is rejected in production. Development sessions
remain dev/test-only.

Tests cover the adapter against a mock WorkOS provider: invited-user login, invitation required, unverified
email rejection, existing-email linking requirement, explicit linking, tampered/expired state, and start being
absent when unconfigured. The real staging sandbox E2E passed on 2026-11-21 (verified Google user,
`ayombarudi@gmail.com`, admin invitation: callback `303`, JIT user, `oidc_identities` mapping, membership,
invitation consumed, session issued, `/v1/auth/me` returns the admin principal). ADR-002 is Accepted.

Sessions are opaque 256-bit tokens; only a SHA-256 hash is stored. Logout revokes the session immediately.
Expired, revoked or cross-organization sessions return `401`. Login and logout append audit events with actor
attribution.

## Documents and object storage (Phase 3)

Migration `0006_documents` adds `documents` and immutable `document_versions` (RLS-scoped). Keys are
server-chosen (`organization/document/version`); clients cannot choose keys.

- `POST /v1/documents` (`documents:write`) — validates kind, content type (`application/pdf`, PNG, JPEG, TIFF,
  CSV), size (≤ 25 MiB) and lowercase hex SHA-256, creates a `pending_upload` document/version, and returns a
  short-lived presigned PUT.
- `POST /v1/documents/{id}/finalize` (`documents:write`) — heads the object, verifies size and SHA-256, then
  transitions to `available`; any mismatch (or missing checksum) quarantines the document.
- `GET /v1/documents/{id}` (`documents:read`) — current version metadata.
- `GET /v1/documents/{id}/download` (`documents:read`) — short-lived presigned GET for available documents.
- `internal/storage` provides the `Store` interface: `Local` (filesystem + HMAC-signed PUT/GET served at
  `/v1/storage/local/object`, development/tests) and `S3` (Cloudflare R2 via the AWS SDK presigner; enforced
  content type and `x-amz-checksum-sha256`). Storage env: `STORAGE_DRIVER` (`local`|`s3`), `STORAGE_LOCAL_DIR`,
  `STORAGE_LOCAL_SECRET`, `STORAGE_PUBLIC_BASE_URL`, `R2_ENDPOINT`, `R2_REGION`, `R2_BUCKET`,
  `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`. Production serving requires `STORAGE_DRIVER=s3` and the R2
  settings.
- `python/atlas_worker.py` is the Python worker skeleton: it claims `document.extract` jobs, writes only
  `job_results`, and never writes canonical tables.

## Reconciliation (Phase 7, in progress)

Migration `0012_reconciliation` adds versioned `reconciliation_runs`, evidence-backed `reconciliation_matches` and
`reconciliation_discrepancies` (RLS-scoped). `internal/reconciliation` implements deterministic two-way header
matching: invoice total vs linked PO totals, currency checks, split invoices across multiple POs, and explicit
`no_linked_po` / `total_mismatch` / `currency_mismatch` discrepancies. Line-level matching now supports partial-quantity allocation across PO lines (FIFO, no over-consumption), with `invoice_line_partial` and `delivery_shortfall` findings. No model authority; runs are reproducible. The REST
trigger and MCP tool create an idempotent `reconciliation.run` operation (honoring `Idempotency-Key`), enqueue
the job, and the worker records success/failure on that operation for status polling.

## Governance and execution (Phase 8, in progress)

Migration `0014_governance` adds versioned `policies`, `approvals`, `operations` and `operation_attempts`
(RLS-scoped). Migration `0015_approval_expiry` adds an `atlas_expire_approvals()` security-definer sweep and a
partial expiry index. `internal/governance` evaluates the latest active policy deny-by-default (`allow_actions`,
`approval_actions`, `amount_threshold_minor`), creates approvals with a policy TTL
(`approval_ttl_minutes`, default 7 days), records decisions/expiry with audit, and keeps operations idempotent.

- `GET/POST /v1/approvals`, `GET /v1/approvals/{id}`, `POST /v1/approvals/{id}/decision`,
  `PUT/GET /v1/policies`, `GET /v1/operations`, `GET /v1/operations/{id}` require `approvals:request`
  (admin/member), `approvals:decide` (admin) or the organization read/manage permissions. Lists accept
  `?status=` and `?limit=` (1–200). There is still no human `actions.execute` permission.
- `StartOperation` requires an approved approval, inserts the operation and enqueues `action.execute` in one
  transaction. Duplicate idempotency keys return the existing operation without enqueueing; reusing a key with
  a different type or payload is rejected. Operation reads include the approval reference and attempt history.
- `internal/execution` claims the operation (pending/running → running), selects an adapter by operation type
  and writes the terminal outcome through governance. Re-delivery of a terminal operation returns the recorded
  result without repeating the effect. Transient adapter errors retry through the job queue; the final attempt
  marks the operation failed. `connector.write` uses `ingestion.ExecuteWrite`, which decrypts the integration
  credential per call and passes the operation ID as the provider idempotency key. The mock connector writes
  only when the integration setting `allow_writes` is true; otherwise the path fails closed.
- `internal/audit` records `operation.started`, `operation.succeeded|failed|cancelled` and
  `approval.expired`. `cmd/atlas-scheduler` runs the expiry sweep every `APPROVAL_EXPIRY_MS` (default 60000).
  Remaining Phase 8 work: UI surfacing.

## Agent access (Phase 9, in progress)

Migration `0016_agent_tokens` adds `agent_tokens` (RLS-scoped) and the
`atlas_resolve_agent_token(text)` security-definer function used for pre-tenant bearer-token resolution.
`internal/agentaccess` implements the agent connection lifecycle.

- `GET /v1/agents`, `POST /v1/agents`, `POST /v1/agents/{id}/revoke`,
  `GET/POST /v1/agents/{id}/tokens`, `POST /v1/agents/{id}/tokens/{tokenID}/revoke` require
  `agents:manage` (admin). `GET /v1/agents/me` reports the resolved principal and scopes to any
  authenticated principal.
- Agent scopes are explicit and wildcard-free: `invoices:read`, `purchase_orders:read`,
  `deliveries:read`, `suppliers:read`, `reconciliation:read`, `reconciliation:run`,
  `discrepancies:read`, `actions:request`, `approvals:request`, `operations:read`.
  `authorization.AuthorizeScope` enforces exact agent scopes; humans map through their role where a
  scope has an equivalent permission.
- Protected routes accept `Authorization: Bearer <token>` (agents) or the existing human session cookie.
  Only token hashes are stored; resolution enforces connection, organization, expiry and revocation state.
  Connection and token create/revoke actions are audited.
- Token issuance here is the development/v0 issuer permitted by the roadmap. Production OAuth issuance and
  MCP authorization metadata remain open under ADR-002/ADR-004/ADR-009.
- Re-run `scripts/provision-app-role.sql`: it grants `atlas_app` EXECUTE on `atlas_resolve_agent_token`
  and revokes `agent_tokens` from `atlas_worker`.

## Shared query layer (Phase 9, in progress)

`internal/query` is the read layer shared by REST (and, next, MCP): invoice/PO search with supplier, number,
currency and amount filters; supplier search; delivery lookup by invoice or PO; discrepancy search by kind,
severity, invoice, run or supplier; plus detail reads delegated to `canonical` and `reconciliation`. All
queries are organization-scoped and capped at 100 rows (default 25).

- Scope-protected REST endpoints: `GET /v1/invoices`, `GET /v1/invoices/{id}`,
  `GET /v1/purchase-orders`, `GET /v1/purchase-orders/{id}`, `GET /v1/suppliers`,
  `GET /v1/suppliers/{id}`, `GET /v1/deliveries?invoice_id=|purchase_order_id=`,
  `GET /v1/discrepancies`. Existing reconciliation, operations and approval-read endpoints now use the same
  scope middleware.
- Humans are authorized by the equivalent role permission; agents need the matching scope. Invalid UUIDs,
  currencies, severities and limits return 400 before any database work.

## MCP endpoint (Phase 9, in progress)

`cmd/atlas-mcp` serves the remote MCP endpoint over Streamable HTTP at `POST /mcp` (JSON responses; GET/DELETE
return 405 for now). It requires an Atlas agent bearer token and returns `401` with `WWW-Authenticate` when
authentication fails. Supported protocol versions are `2024-11-05`, `2025-03-26` and `2025-06-18`; the server
negotiates the client's requested version and pins its default.

- Methods: `initialize`, `ping`, `notifications/initialized`, `tools/list`, `tools/call`,
  `resources/list`, `resources/templates/list`, `resources/read`.
- Transport: JSON responses by default; when the client sends `Accept: text/event-stream`, the single
  JSON-RPC response is framed as an SSE `message` event. `initialize` returns an optional `MCP-Session-Id`;
  subsequent requests may send it, `DELETE` terminates it, unknown ids return 404 and a session used by a
  different agent returns 403. Sessions are in-process (stateless requests remain supported), so multi-instance
  deployments need sticky routing or should stay stateless. `tools/call` requests with
  `params._meta.progressToken` and an SSE `Accept` receive `notifications/progress` events before and after the
  response.
- Eleven tools: `search_invoices`, `get_invoice`, `search_purchase_orders`, `get_delivery_records`,
  `get_supplier`, `reconcile_invoice`, `get_reconciliation`, `search_discrepancies`,
  `request_supplier_followup`, `request_approval`, `get_operation_status`. Each tool declares its scope;
  missing scope returns JSON-RPC `-32003`, unknown tools `-32602`, unknown methods `-32601`.
- Tools share `internal/query`, `reconciliation`, `governance` and `jobs` with REST; handlers only validate
  transport input and map results. `request_supplier_followup` evaluates policy and records an agent-attributed
  approval. `reconcile_invoice` creates an idempotent `reconciliation.run` operation, enqueues the job and
  returns the operation id; the worker records the run result on that operation, so `get_operation_status`
  works for reconciliation.
- Resources are template-based: `atlas://organizations/{org}/invoices/{invoiceId}` and
  `atlas://organizations/{org}/reconciliations/{runId}`. Reads enforce the principal's organization and the
  corresponding scope; `resources/list` returns no materialized resources.
- REST parity for follow-ups: `POST /v1/invoices/{invoiceID}/supplier-followups` (scope `actions:request`).
- `internal/mcpapi/testdata/tools.json` snapshots the eleven tool schemas; regenerate with
  `go test ./internal/mcpapi -run TestToolSchemasSnapshot -update`.
- Config: `DATABASE_URL`, `MCP_PORT` (falls back to `PORT`, default `8081`), `LOG_LEVEL`, `ATLAS_ENV`. The
  command refuses `ATLAS_ENV=production` until ADR-002/ADR-004 production token issuance exists.
- `TestRESTAndMCPParity` asserts identical invoice/discrepancy payloads over REST and MCP;
  `TestMCPReconciliationCorpusRegression` runs every fixture scenario through the MCP tool, worker
  execution, operation-status and reconciliation reads.
- Not yet implemented: the ADR-009 client matrix and live client validation.

## Canonical model and ontology (Phase 6, complete)

Migration `0009_canonical` adds the v0 canonical schema (`suppliers`, `purchase_orders`, `po_lines`, `invoices`,
`invoice_lines`, `delivery_records`, `delivery_lines`, `invoice_po_links`, `source_entity_links`) plus
`ontology_versions` v1. Money is `bigint` minor units + 3-letter ISO currency; quantities are
`numeric(18,6)`; business numbers are unique per organization; invoice?PO is explicitly many-to-many.
`internal/canonical` provides org-scoped writes (`CreateSupplier`, `CreatePurchaseOrder`, `CreateInvoice`,
`LinkInvoicePO`), evidence-backed `LinkSource` (method, confidence, evidence) and counts/ontology access.
Every canonical table is RLS-scoped and writes validate the active ontology. Line-level writes, idempotent
projection from normalized records with provenance, and invoice read models (`GetInvoiceDetail`) are
implemented, along with delivery/partial-delivery writes and PO/supplier read models. The JSON fixture corpus (split-invoice and
partial-delivery scenarios) and fail-closed ontology validation (single active version, required
entities/relationships) are implemented and tested.

## Connectors and ingestion (Phase 4)

Migration `0007_connectors` adds integration settings, `integration_credentials` (envelope-encrypted),
`sync_runs`, `sync_cursors`, immutable `source_records` (raw payload as `bytea` + SHA-256), `normalized_records`
(`mapping_version`) and `provenance_records` — all RLS-scoped.

- `internal/connectors` defines the contract (`ValidateConnection`, `DiscoverCapabilities`, `Sync`,
  `FetchDocument`, `ExecuteWrite`), a thread-safe registry, a deterministic `mock` adapter (suppliers, POs,
  deliveries, invoices with pagination) and the upload-driven `document_inbox` adapter. Adapters only translate
  provider shapes; they contain no canonical logic.
- `internal/ingestion` runs syncs: validates the connection, pages objects within a limit, persists raw source
  records idempotently (`UNIQUE(organization, integration, object_type, external_id)`), applies the versioned
  normalization (`mock-v1`), writes provenance, advances cursors and records `sync_runs`; the integration health
  becomes `degraded` on failure.
- `internal/secrets` implements envelope encryption (AES-256-GCM data key per credential, master key wraps it)
  bound to organization + integration; `ATLAS_SECRET_KEY` (32 bytes base64/hex) enables the worker's
  `connector.sync` job handler, payload `{"organization_id": "...", "integration_id": "..."}`.
- Management endpoints: `POST/GET /v1/integrations`, credentials, `/sync` (enqueues `connector.sync`), plus
  `POST /v1/webhooks/{organization}/{integration}` (HMAC-SHA256 over `timestamp.body`, 5-minute window, dedupe
  by event id, raw persistence, targeted sync). Finalizing a document records it through the document-inbox
  pipeline. Handling later: provider-specific rate-limit retry/token-refresh orchestration.

## Jobs, outbox and workers (Phase 3)

Migration `0005_jobs_outbox` adds `jobs`, `job_attempts`, `job_results` and `outbox_events` (all RLS-scoped to
`app.organization_id`). Execution semantics are **at-least-once**, so handlers must be idempotent using
`idempotency_key` (unique per organization + type).

- `internal/jobs.NewQueue(pool)` — transactional enqueue (`Enqueue`), atomic claim (`Claim`, via the
  `atlas_claim_job` security-definer function using `FOR UPDATE SKIP LOCKED`), `Complete`, `Fail`
  (capped exponential backoff with jitter, non-retryable → dead-letter), `ReapExpiredLeases`, `Requeue`, and
  the outbox (`EnqueueEvent` in the business transaction, `Dispatch`).
- Lease-expired jobs are reaped by `atlas_reap_expired_leases`; the abandoned attempt is recorded as
  `lease_expired`. Outbox rows are claimed with `atlas_claim_outbox`; dedupe keys make retries harmless.
- `cmd/atlas-worker` claims jobs, runs registered handlers (`system.noop`, `reconciliation.run`,
  `document.validate`, `connector.sync`, `action.execute`) and records results. `cmd/atlas-scheduler`
  dispatches outbox events to jobs, reaps leases and sweeps expired approvals.
- Worker/scheduler env: `WORKER_DATABASE_URL` (fell back to `DATABASE_URL`), `WORKER_ID`, `JOB_LEASE_SECONDS`,
  `JOB_POLL_MS`, `OUTBOX_POLL_MS`, `LEASE_REAP_MS`, `APPROVAL_EXPIRY_MS`. Cross-organization queue/approval
  access is only through the security-definer functions granted to `atlas_worker`.

## Deployment (ADR-001: AWS)

ADR-001 selects AWS. `backend/Dockerfile` builds every Go process with
`--build-arg SERVICE=atlas-api|atlas-mcp|atlas-worker|atlas-scheduler|atlas-migrate|atlas-provision`;
`backend/Dockerfile.python` packages the extraction worker. Images run as nonroot and include
`/app/migrations` + `/app/scripts`. The full ECS/RDS/ALB/Secrets Manager runbook, environment matrix, deploy
order and sandbox notes are in [`deploy/aws.md`](deploy/aws.md). Per-process IAM task roles, database-role
mapping and secret rotation are in [`deploy/iam.md`](deploy/iam.md) (ADR-004). The MCP service exposes
`GET /healthz` for ALB health checks. The sandbox uses `eu-west-1`; the production region remains under ADR-010.

## Row-Level Security and database roles

Tenant tables (`organizations`, `memberships`, `agent_connections`, `agent_scopes`, `integrations`,
`integration_credentials`, `sync_runs`, `sync_cursors`, `source_records`, `normalized_records`,
`provenance_records`, `audit_events`, `documents`, `document_versions`, `jobs`, `job_attempts`, `job_results`,
`outbox_events`, `policies`, `approvals`, `operations`, `operation_attempts`, `agent_tokens`) use PostgreSQL RLS keyed on
`app.organization_id`, set per transaction by
`internal/tenancy`. `users` is a global identity record and `sessions` is read before an organization context
exists; both remain intentionally **not** RLS-protected. They are protected by separated database roles and a
narrow read function instead:

- `atlas_app` (tenant runtime) has **no** privileges on `users` or `sessions`. Member listing goes through the
  security-definer function `atlas_list_members()`, which filters by the current organization setting.
- `atlas_auth` (identity/session operations) may read `users`, `organizations`, `memberships` and manage
  `sessions`; it is used only by the identity service via `AUTH_DATABASE_URL` and cannot mutate tenant data.
- `atlas_worker` (background jobs) has tenant DML but no `users`/`sessions` access.
- `atlas_migrator` owns schema objects and runs migrations; runtime roles are never owners and never bypass RLS.

Provision roles after migrations. The SQL file is plain SQL (no psql meta-commands) and can be executed by any
client; the Atlas Go tool supplies the passwords through session settings:

```bash
ATLAS_APP_PASSWORD="$ATLAS_APP_PASSWORD" \
ATLAS_AUTH_PASSWORD="$ATLAS_AUTH_PASSWORD" \
ATLAS_MIGRATOR_PASSWORD="$ATLAS_MIGRATOR_PASSWORD" \
ATLAS_WORKER_PASSWORD="$ATLAS_WORKER_PASSWORD" \
go run ./cmd/atlas-provision
```

Or with psql:

```bash
psql "$DATABASE_URL" \
  -c "SET atlas.app_password = '$ATLAS_APP_PASSWORD';
      SET atlas.auth_password = '$ATLAS_AUTH_PASSWORD';
      SET atlas.migrator_password = '$ATLAS_MIGRATOR_PASSWORD';
      SET atlas.worker_password = '$ATLAS_WORKER_PASSWORD';" \
  -f scripts/provision-app-role.sql
```

The script creates all four roles with `NOSUPERUSER NOBYPASSRLS`, transfers table/function ownership to the
migrator, grants tenant DML to app/worker, revokes direct `users`/`sessions` access from app/worker, revokes
`UPDATE`/`DELETE` on `audit_events`, and grants `atlas_app` only `EXECUTE` on `atlas_list_members()`. Re-run
after applying migrations so new tables receive grants. Never run the API as the schema owner or a superuser.

Integration tests execute **this exact SQL file** (via pgx) before every tenant test, then assert role
attributes, object ownership and cross-tenant privilege boundaries. The same tests run in CI against
PostgreSQL 16.

Verify everything with the portable scripts:

```bash
./scripts/check.sh          # bash
powershell -File scripts/check.ps1   # Windows PowerShell
```

## Migrations

- Files are named `NNNN_name.up.sql` and `NNNN_name.down.sql`, with a numeric prefix (zero-padded by
  convention). The runner sorts by numeric value, so `10` runs after `2`; this ordering is covered by tests.
- Each applied migration is checksum-verified; editing an applied migration fails the next `up`.
- Migrations are advisory-locked, transactional and reversible. `atlas-migrate down -steps N` reverts.
- `0001_platform_metadata` provides a platform-scoped table for the sqlc baseline. `0002_identity_tenancy`
  creates organizations, users, memberships, sessions, agent/integration principals, audit events and the RLS
  policies described above.

## sqlc (temporary Phase 1 configuration)

`sqlc.yaml` currently points `schema` at `migrations/0001_platform_metadata.up.sql` because the canonical
schema does not exist yet. This is a **temporary implementation choice**:

- It must be replaced with the complete canonical schema when the canonical data model is introduced in
  **Phase 5** (see `roadmap.md`).
- Until then, sqlc generates only the platform-metadata query. Identity/tenancy repositories use reviewed SQL
  directly against their own tables.
- Future migrations must be parsed in numeric order; the migration loader already enforces this.
- Generated code lives in `internal/platform/db/gen` and must be regenerated with `sqlc generate`, never
  hand-edited. CI verifies regeneration produces no diff.

## Tests

```bash
go test ./cmd/... ./internal/... -count=1
```

- Integration tests use `TEST_DATABASE_URL` when set; otherwise they start a disposable embedded PostgreSQL
  16 instance and reset its `public` schema. They never touch a shared database.
- `TestPostgresMajorVersionParity` asserts the server is PostgreSQL 16.x, matching the CI `postgres:16`
  service.
- `TestMigrationLifecycle`, `TestMigrationsApplyInOrder` and the checksum-tamper case verify migration
  consistency and ordering.
- `TestAPIBinaryStartsAndServes` builds and runs the real `atlas-api` binary; `TestMigrateCLI` exercises the
  migration CLI.
- Phase 2: `TestAuthenticationAndAuthorization` (dev login, principal, RBAC denial, logout),
  `TestCrossTenantLoginRejected`, `TestRLSTenantIsolation`, `TestRLSContextIsolationUnderConcurrency`,
  `TestAuditAttributionAndImmutability`, `TestExpiredSessionRejected`,
  `TestDevelopmentLoginDisabled`, and `TestProvisioningSQLRolePrivileges` (executes the real provisioning SQL
  and asserts role attributes, ownership, app/auth/worker privilege boundaries, and member-function scoping).
- The race detector is mandatory in Linux CI (`go test -race`). It is optional locally on Windows; enabling it
  would require a C toolchain, which is intentionally not added as a project dependency.













