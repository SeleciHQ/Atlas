-- ADR-008: FX rate provenance. Rates are recorded as dated snapshots; source
-- distinguishes ECB reference data from manual/organization overrides.
ALTER TABLE fx_rates
    ADD COLUMN source text NOT NULL DEFAULT 'manual'
    CHECK (source IN ('manual', 'ecb', 'organization'));
CREATE INDEX fx_rates_source ON fx_rates (organization_id, source, effective_at DESC);
