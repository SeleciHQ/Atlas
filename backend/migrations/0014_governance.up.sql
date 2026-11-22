-- Phase 8: central policy, approvals and idempotent governed operations.

CREATE TABLE policies (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, name, version)
);

CREATE TABLE approvals (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    operation_type text NOT NULL,
    object_type text NOT NULL,
    object_id text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'expired')),
    requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
    decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
    decision_reason text,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    PRIMARY KEY (organization_id, id)
);
CREATE INDEX approvals_status ON approvals (organization_id, status, created_at);

CREATE TABLE operations (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    approval_id uuid,
    type text NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    idempotency_key text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    result jsonb,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, idempotency_key),
    FOREIGN KEY (organization_id, approval_id) REFERENCES approvals (organization_id, id) ON DELETE SET NULL
);

CREATE TABLE operation_attempts (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    operation_id uuid NOT NULL,
    attempt integer NOT NULL CHECK (attempt > 0),
    status text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    external_reference text,
    error text,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, operation_id) REFERENCES operations (organization_id, id) ON DELETE CASCADE
);

ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE approvals ENABLE ROW LEVEL SECURITY;
ALTER TABLE operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE operation_attempts ENABLE ROW LEVEL SECURITY;
CREATE POLICY policies_tenant ON policies FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY approvals_tenant ON approvals FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY operations_tenant ON operations FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY operation_attempts_tenant ON operation_attempts FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
