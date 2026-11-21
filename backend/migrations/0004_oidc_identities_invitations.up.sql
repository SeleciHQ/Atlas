-- Phase 2.1: external identities and organization invitations.

CREATE TABLE oidc_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer text NOT NULL,
    subject text NOT NULL,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email_at_link text,
    email_verified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz,
    UNIQUE (issuer, subject)
);
CREATE INDEX oidc_identities_user_idx ON oidc_identities (user_id);

CREATE TABLE organization_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email text NOT NULL,
    role text NOT NULL CHECK (role IN ('admin', 'member')),
    token_hash text NOT NULL UNIQUE,
    invited_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    accepted_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX organization_invitations_org_idx ON organization_invitations (organization_id, lower(email));
CREATE UNIQUE INDEX organization_invitations_pending_idx
    ON organization_invitations (organization_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

ALTER TABLE organization_invitations ENABLE ROW LEVEL SECURITY;

CREATE POLICY organization_invitations_select ON organization_invitations FOR SELECT
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY organization_invitations_insert ON organization_invitations FOR INSERT
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY organization_invitations_update ON organization_invitations FOR UPDATE
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY organization_invitations_delete ON organization_invitations FOR DELETE
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);

-- oidc_identities is a global identity mapping (pre-tenant), like users and
-- sessions: it is protected by database grants, not RLS.

-- Narrow helper for the identity role: resolve a user's active memberships
-- without an organization context (used only by the OIDC login path).
CREATE OR REPLACE FUNCTION atlas_memberships_for_user(p_user uuid)
RETURNS TABLE (organization_id uuid, role text, organization_status text)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT m.organization_id, m.role, o.status
    FROM memberships m JOIN organizations o ON o.id = m.organization_id
    WHERE m.user_id = p_user AND o.status = 'active'
$$;
REVOKE ALL ON FUNCTION atlas_memberships_for_user(uuid) FROM PUBLIC;

