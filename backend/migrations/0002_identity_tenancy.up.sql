-- Phase 2: identity, tenancy and security foundation.
--
-- Tenant-scoped tables carry organization_id and have Row-Level Security.
-- `users` is a global identity record (a user may belong to several
-- organizations) and `sessions` is read before an organization context exists;
-- both are intentionally not RLS-protected and are access-controlled by the
-- application/DB role grants instead.
-- `audit_events` is append-only: the application role is granted INSERT/SELECT
-- but not UPDATE/DELETE (see backend/README.md role provisioning).

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    display_name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(email));

CREATE TABLE memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('admin', 'finance_ops', 'controller', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);
CREATE INDEX sessions_org_user_idx ON sessions (organization_id, user_id);

CREATE TABLE agent_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    UNIQUE (organization_id, name)
);

CREATE TABLE agent_scopes (
    agent_connection_id uuid NOT NULL REFERENCES agent_connections (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope text NOT NULL,
    PRIMARY KEY (agent_connection_id, scope)
);
CREATE INDEX agent_scopes_org_idx ON agent_scopes (organization_id);

CREATE TABLE integrations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    provider text NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'connected' CHECK (status IN ('connected', 'degraded', 'action_required', 'disconnected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, provider, name)
);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    actor_kind text NOT NULL CHECK (actor_kind IN ('human', 'agent', 'integration', 'workload', 'system')),
    actor_id text,
    actor_display text,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    decision text NOT NULL DEFAULT 'recorded' CHECK (decision IN ('allow', 'deny', 'require_approval', 'recorded')),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_org_created_idx ON audit_events (organization_id, created_at DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_kind, actor_id);

-- Row-Level Security for tenant-owned tables.
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_scopes ENABLE ROW LEVEL SECURITY;
ALTER TABLE integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY organizations_select ON organizations FOR SELECT
    USING (id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY organizations_update ON organizations FOR UPDATE
    USING (id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (id = nullif(current_setting('app.organization_id', true), '')::uuid);

CREATE POLICY memberships_select ON memberships FOR SELECT
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY memberships_insert ON memberships FOR INSERT
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY memberships_update ON memberships FOR UPDATE
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY memberships_delete ON memberships FOR DELETE
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

CREATE POLICY agent_connections_tenant ON agent_connections FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

CREATE POLICY agent_scopes_tenant ON agent_scopes FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

CREATE POLICY integrations_tenant ON integrations FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

CREATE POLICY audit_events_select ON audit_events FOR SELECT
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY audit_events_insert ON audit_events FOR INSERT
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
