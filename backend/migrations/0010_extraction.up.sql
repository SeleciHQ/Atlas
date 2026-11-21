-- Phase 6: extraction drafts (validated extraction results; never canonical).

CREATE TABLE extraction_drafts (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL,
    provider text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    status text NOT NULL CHECK (status IN ('validated', 'rejected')),
    confidence numeric(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    payload jsonb NOT NULL,
    warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, document_id) REFERENCES documents (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX extraction_drafts_document ON extraction_drafts (organization_id, document_id, created_at DESC);

ALTER TABLE extraction_drafts ENABLE ROW LEVEL SECURITY;
CREATE POLICY extraction_drafts_tenant ON extraction_drafts FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
