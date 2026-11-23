-- Phase 9: agent connection bearer tokens. Only hashes are stored. Resolution
-- happens before an organization context exists, so it goes through a
-- security-definer function that enforces connection, organization and expiry
-- state and returns the connection's scopes.

-- Tenant-qualified foreign key target for agent_tokens.
ALTER TABLE agent_connections
    ADD CONSTRAINT agent_connections_org_id_key UNIQUE (organization_id, id);

CREATE TABLE agent_tokens (
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    agent_connection_id uuid NOT NULL,
    token_hash text NOT NULL UNIQUE,
    name text NOT NULL DEFAULT 'default',
    issued_by uuid REFERENCES users (id) ON DELETE SET NULL,
    expires_at timestamptz,
    revoked_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, agent_connection_id)
        REFERENCES agent_connections (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX agent_tokens_connection ON agent_tokens (organization_id, agent_connection_id, created_at DESC);

ALTER TABLE agent_tokens ENABLE ROW LEVEL SECURITY;
CREATE POLICY agent_tokens_tenant ON agent_tokens FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

CREATE FUNCTION atlas_resolve_agent_token(p_hash text)
RETURNS TABLE (organization_id uuid, agent_connection_id uuid, name text, scopes text[])
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
BEGIN
    RETURN QUERY
    WITH matched AS (
        SELECT t.organization_id AS org_id,
               t.agent_connection_id AS connection_id,
               c.name AS connection_name,
               coalesce(array_agg(s.scope ORDER BY s.scope)
                        FILTER (WHERE s.scope IS NOT NULL), ARRAY[]::text[]) AS scope_list
        FROM agent_tokens t
        JOIN agent_connections c
          ON c.organization_id = t.organization_id AND c.id = t.agent_connection_id
        JOIN organizations o ON o.id = t.organization_id
        LEFT JOIN agent_scopes s
          ON s.organization_id = c.organization_id AND s.agent_connection_id = c.id
        WHERE t.token_hash = p_hash
          AND t.revoked_at IS NULL
          AND (t.expires_at IS NULL OR t.expires_at > now())
          AND c.status = 'active'
          AND o.status = 'active'
        GROUP BY t.organization_id, t.agent_connection_id, c.name
    ), touched AS (
        UPDATE agent_tokens SET last_used_at = now()
        WHERE token_hash = p_hash AND EXISTS (SELECT 1 FROM matched)
    )
    SELECT org_id, connection_id, connection_name, scope_list FROM matched;
END;
$$;

REVOKE ALL ON FUNCTION atlas_resolve_agent_token(text) FROM PUBLIC;
