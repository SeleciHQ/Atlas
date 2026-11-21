-- Phase 4: inbound webhook events (raw persistence, dedupe, targeted sync).

CREATE TABLE webhook_events (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    integration_id uuid NOT NULL,
    external_event_id text NOT NULL,
    event_type text NOT NULL DEFAULT '',
    raw_payload bytea NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, integration_id, external_event_id),
    FOREIGN KEY (organization_id, integration_id) REFERENCES integrations (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX webhook_events_integration ON webhook_events (organization_id, integration_id, received_at DESC);

ALTER TABLE webhook_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY webhook_events_tenant ON webhook_events FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
