-- Phase 3: private documents and immutable versions.

CREATE TABLE documents (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('invoice', 'credit_note', 'purchase_order', 'delivery_note', 'contract', 'other')),
    status text NOT NULL DEFAULT 'pending_upload'
        CHECK (status IN ('pending_upload', 'available', 'quarantined', 'deleted')),
    current_version integer NOT NULL DEFAULT 1 CHECK (current_version > 0),
    retention_class text NOT NULL DEFAULT 'standard' CHECK (retention_class IN ('standard', 'financial', 'legal_hold')),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id)
);
CREATE INDEX documents_organization_status ON documents (organization_id, status, created_at DESC);

CREATE TABLE document_versions (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    sha256 text NOT NULL CHECK (length(sha256) = 64),
    storage_key text NOT NULL,
    status text NOT NULL DEFAULT 'pending_upload'
        CHECK (status IN ('pending_upload', 'available', 'quarantined')),
    uploaded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, document_id, version),
    UNIQUE (organization_id, storage_key),
    FOREIGN KEY (organization_id, document_id) REFERENCES documents (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX document_versions_document ON document_versions (organization_id, document_id, version DESC);

ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_versions ENABLE ROW LEVEL SECURITY;

CREATE POLICY documents_tenant ON documents FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY document_versions_tenant ON document_versions FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
