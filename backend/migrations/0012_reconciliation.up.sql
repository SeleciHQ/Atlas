-- Phase 7: deterministic reconciliation runs, matches and discrepancies.

CREATE TABLE reconciliation_runs (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    invoice_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    engine_version integer NOT NULL DEFAULT 1 CHECK (engine_version > 0),
    summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES invoices (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX reconciliation_runs_invoice ON reconciliation_runs (organization_id, invoice_id, created_at DESC);

CREATE TABLE reconciliation_matches (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL,
    match_type text NOT NULL CHECK (match_type IN ('header', 'line')),
    invoice_id uuid NOT NULL,
    purchase_order_id uuid NOT NULL,
    confidence numeric(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, run_id) REFERENCES reconciliation_runs (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, invoice_id) REFERENCES invoices (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE reconciliation_discrepancies (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL,
    kind text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('info', 'warning', 'error')),
    description text NOT NULL,
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, run_id) REFERENCES reconciliation_runs (organization_id, id) ON DELETE CASCADE
);

ALTER TABLE reconciliation_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE reconciliation_matches ENABLE ROW LEVEL SECURITY;
ALTER TABLE reconciliation_discrepancies ENABLE ROW LEVEL SECURITY;
CREATE POLICY reconciliation_runs_tenant ON reconciliation_runs FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY reconciliation_matches_tenant ON reconciliation_matches FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
CREATE POLICY reconciliation_discrepancies_tenant ON reconciliation_discrepancies FOR ALL
    USING (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid)
    WITH CHECK (organization_id = nullif(current_setting('app.organization_id', true), '')::uuid);
