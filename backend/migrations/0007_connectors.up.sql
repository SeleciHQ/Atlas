-- Phase 4: connector integrations, credentials, sync history, source and
-- normalized records with provenance.

ALTER TABLE integrations ADD COLUMN kind text NOT NULL DEFAULT 'mock'
    CHECK (kind IN ('mock', 'document_inbox'));
ALTER TABLE integrations ADD COLUMN config jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE integrations ADD COLUMN last_synced_at timestamptz;
ALTER TABLE integrations ADD CONSTRAINT integrations_organization_id_id_key UNIQUE (organization_id, id);

CREATE TABLE integration_credentials (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL,
    ciphertext bytea NOT NULL,
    wrapped_key bytea NOT NULL,
    nonce bytea NOT NULL,
    wrapped_nonce bytea NOT NULL,
    key_version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    rotated_at timestamptz,
    PRIMARY KEY (organization_id, integration_id),
    FOREIGN KEY (organization_id, integration_id) REFERENCES integrations (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE sync_runs (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL,
    object_type text,
    status text NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed', 'partial')),
    cursor_before text,
    cursor_after text,
    records_seen integer NOT NULL DEFAULT 0,
    records_written integer NOT NULL DEFAULT 0,
    error text,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, integration_id) REFERENCES integrations (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX sync_runs_integration ON sync_runs (organization_id, integration_id, started_at DESC);

CREATE TABLE sync_cursors (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL,
    object_type text NOT NULL,
    cursor text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, integration_id, object_type),
    FOREIGN KEY (organization_id, integration_id) REFERENCES integrations (organization_id, id) ON DELETE CASCADE
);

-- Raw payloads are stored byte-for-byte (bytea), never re-encoded.
CREATE TABLE source_records (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL,
    provider text NOT NULL,
    object_type text NOT NULL,
    external_id text NOT NULL,
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    raw_payload bytea NOT NULL,
    checksum text NOT NULL CHECK (length(checksum) = 64),
    batch_ref text,
    received_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, integration_id, object_type, external_id),
    FOREIGN KEY (organization_id, integration_id) REFERENCES integrations (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX source_records_integration ON source_records (organization_id, integration_id, object_type, received_at DESC);

CREATE TABLE normalized_records (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    source_record_id uuid NOT NULL,
    record_type text NOT NULL,
    schema_version integer NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    mapping_version text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, source_record_id, mapping_version),
    FOREIGN KEY (organization_id, source_record_id) REFERENCES source_records (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE provenance_records (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    source_record_id uuid NOT NULL,
    transformation text NOT NULL,
    mapping_version text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, source_record_id) REFERENCES source_records (organization_id, id) ON DELETE CASCADE
);

ALTER TABLE integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE integration_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_cursors ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE normalized_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE provenance_records ENABLE ROW LEVEL SECURITY;

CREATE POLICY integration_credentials_tenant ON integration_credentials FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY sync_runs_tenant ON sync_runs FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY sync_cursors_tenant ON sync_cursors FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY source_records_tenant ON source_records FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY normalized_records_tenant ON normalized_records FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY provenance_records_tenant ON provenance_records FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
