-- Phase 2 security review hardening.
--
-- 1. Human membership roles are reduced to two built-in roles: admin and
--    member. Departmental job titles are not baked into authorization;
--    permissions remain the mechanism (approval permissions are granted
--    explicitly in later phases).
-- 2. Member listing uses a narrowly scoped security-definer function so the
--    runtime application role does not need direct access to the global users
--    table.

ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_check;
UPDATE memberships SET role = 'member' WHERE role IN ('finance_ops', 'controller', 'viewer');
ALTER TABLE memberships ADD CONSTRAINT memberships_role_check CHECK (role IN ('admin', 'member'));

CREATE OR REPLACE FUNCTION atlas_list_members()
RETURNS TABLE (
    membership_id uuid,
    user_id uuid,
    display_name text,
    email text,
    role text,
    created_at timestamptz
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT m.id, u.id, u.display_name, u.email, m.role, m.created_at
    FROM memberships m
    JOIN users u ON u.id = m.user_id
    WHERE m.organization_id = nullif(current_setting('app.organization_id', true), '')::uuid
    ORDER BY u.display_name ASC
$$;

REVOKE ALL ON FUNCTION atlas_list_members() FROM PUBLIC;
