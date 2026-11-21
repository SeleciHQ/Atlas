DROP FUNCTION IF EXISTS atlas_list_members();

ALTER TABLE memberships DROP CONSTRAINT IF EXISTS memberships_role_check;
UPDATE memberships SET role = 'viewer' WHERE role = 'member';
ALTER TABLE memberships ADD CONSTRAINT memberships_role_check CHECK (role IN ('admin', 'finance_ops', 'controller', 'viewer'));
