-- Phase 6: supplier identity evidence for candidate strength.

ALTER TABLE suppliers ADD COLUMN email text;
ALTER TABLE suppliers ADD COLUMN vat_id text;
CREATE UNIQUE INDEX suppliers_email_unique ON suppliers (organization_id, lower(email)) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX suppliers_vat_unique ON suppliers (organization_id, vat_id) WHERE vat_id IS NOT NULL;
