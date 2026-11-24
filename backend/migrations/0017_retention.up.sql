-- ADR-007: retention/legal-hold metadata. Retention defaults live in code;
-- this table records governed holds that block deletion of a resource.

CREATE TABLE legal_holds (
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    resource_type text NOT NULL CHECK (resource_type IN (
        'document', 'source_record', 'canonical_invoice', 'canonical_purchase_order',
        'reconciliation_run', 'audit_event', 'organization_data')),
    resource_id text NOT NULL CHECK (length(trim(resource_id)) > 0),
    reason text NOT NULL CHECK (length(trim(reason)) > 0),
    created_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    released_by uuid REFERENCES users (id) ON DELETE SET NULL,
    released_at timestamptz,
    PRIMARY KEY (organization_id, id)
);
CREATE UNIQUE INDEX legal_holds_one_active
    ON legal_holds (organization_id, resource_type, resource_id)
    WHERE released_at IS NULL;
CREATE INDEX legal_holds_active_lookup
    ON legal_holds (organization_id, resource_type, resource_id, created_at DESC)
    WHERE released_at IS NULL;

ALTER TABLE legal_holds ENABLE ROW LEVEL SECURITY;
CREATE POLICY legal_holds_tenant ON legal_holds FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
