DROP FUNCTION IF EXISTS atlas_resolve_agent_token(text);
DROP TABLE IF EXISTS agent_tokens;
ALTER TABLE agent_connections DROP CONSTRAINT IF EXISTS agent_connections_org_id_key;
