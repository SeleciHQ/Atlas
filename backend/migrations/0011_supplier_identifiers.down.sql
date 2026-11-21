DROP INDEX IF EXISTS suppliers_vat_unique;
DROP INDEX IF EXISTS suppliers_email_unique;
ALTER TABLE suppliers DROP COLUMN IF EXISTS vat_id;
ALTER TABLE suppliers DROP COLUMN IF EXISTS email;
