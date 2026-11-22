-- Phase 7: organization-scoped FX rates and unit-of-measure factors.
CREATE TABLE fx_rates (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    base_currency text NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'),
    quote_currency text NOT NULL CHECK (quote_currency ~ '^[A-Z]{3}$'),
    rate numeric(18,8) NOT NULL CHECK (rate > 0),
    effective_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, base_currency, quote_currency, effective_at)
);
CREATE TABLE uom_conversions (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    from_unit text NOT NULL,
    to_unit text NOT NULL,
    factor numeric(18,8) NOT NULL CHECK (factor > 0),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, from_unit, to_unit)
);
ALTER TABLE fx_rates ENABLE ROW LEVEL SECURITY;
ALTER TABLE uom_conversions ENABLE ROW LEVEL SECURITY;
CREATE POLICY fx_rates_tenant ON fx_rates FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY uom_conversions_tenant ON uom_conversions FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
