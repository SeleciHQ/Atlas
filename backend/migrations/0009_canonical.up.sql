-- Phase 5: canonical v0 model, ontology v1, and source-to-entity links.
-- Money is integer minor units + ISO-4217 currency; quantities are numeric(18,6).

CREATE TABLE ontology_versions (
    version integer PRIMARY KEY,
    status text NOT NULL CHECK (status IN ('active', 'deprecated')),
    definitions jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    activated_at timestamptz
);
INSERT INTO ontology_versions (version, status, definitions, activated_at)
VALUES (1, 'active', '{
  "entities": ["supplier", "invoice", "invoice_line", "purchase_order", "po_line",
               "delivery_record", "delivery_line", "document"],
  "relationships": ["invoice_po_link", "line_match", "delivery_fulfillment", "source_entity_link"]
}'::jsonb, now());

CREATE TABLE suppliers (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(trim(name)) > 0),
    legal_name text,
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    ontology_version integer NOT NULL DEFAULT 1 REFERENCES ontology_versions(version),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, name)
);

CREATE TABLE purchase_orders (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    supplier_id uuid NOT NULL,
    number text NOT NULL,
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    total_minor bigint NOT NULL CHECK (total_minor >= 0),
    ordered_on date,
    ontology_version integer NOT NULL DEFAULT 1 REFERENCES ontology_versions(version),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, number),
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id)
);

CREATE TABLE po_lines (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    purchase_order_id uuid NOT NULL,
    line_number integer NOT NULL CHECK (line_number > 0),
    description text NOT NULL DEFAULT '',
    quantity numeric(18,6) NOT NULL CHECK (quantity > 0),
    unit text NOT NULL DEFAULT 'unit',
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, purchase_order_id, line_number),
    FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE invoices (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    supplier_id uuid NOT NULL,
    number text NOT NULL,
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    total_minor bigint NOT NULL CHECK (total_minor >= 0),
    issued_on date,
    due_on date,
    status text NOT NULL DEFAULT 'recorded' CHECK (status IN ('draft', 'recorded', 'void')),
    ontology_version integer NOT NULL DEFAULT 1 REFERENCES ontology_versions(version),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, number),
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id)
);

CREATE TABLE invoice_lines (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    invoice_id uuid NOT NULL,
    line_number integer NOT NULL CHECK (line_number > 0),
    description text NOT NULL DEFAULT '',
    quantity numeric(18,6) NOT NULL CHECK (quantity > 0),
    unit text NOT NULL DEFAULT 'unit',
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, invoice_id, line_number),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES invoices (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE delivery_records (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    supplier_id uuid NOT NULL,
    purchase_order_id uuid,
    number text NOT NULL,
    delivered_on date,
    ontology_version integer NOT NULL DEFAULT 1 REFERENCES ontology_versions(version),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    UNIQUE (organization_id, number),
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id),
    FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id)
);

CREATE TABLE delivery_lines (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    delivery_record_id uuid NOT NULL,
    po_line_id uuid NOT NULL,
    quantity numeric(18,6) NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, delivery_record_id) REFERENCES delivery_records (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, po_line_id) REFERENCES po_lines (organization_id, id)
);

-- Invoice <-> PO is explicitly many-to-many (split invoices, consolidated POs).
CREATE TABLE invoice_po_links (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    invoice_id uuid NOT NULL,
    purchase_order_id uuid NOT NULL,
    allocated_minor bigint CHECK (allocated_minor >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, invoice_id, purchase_order_id),
    FOREIGN KEY (organization_id, invoice_id) REFERENCES invoices (organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id) ON DELETE CASCADE
);

CREATE TABLE source_entity_links (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    source_record_id uuid NOT NULL,
    entity_type text NOT NULL CHECK (entity_type IN ('supplier', 'invoice', 'invoice_line', 'purchase_order', 'po_line', 'delivery_record', 'delivery_line')),
    entity_id uuid NOT NULL,
    method text NOT NULL,
    confidence numeric(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id, source_record_id) REFERENCES source_records (organization_id, id) ON DELETE CASCADE
);
CREATE INDEX source_entity_links_entity ON source_entity_links (organization_id, entity_type, entity_id);

ALTER TABLE suppliers ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE po_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE delivery_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE delivery_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_po_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_entity_links ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['suppliers','purchase_orders','po_lines','invoices','invoice_lines',
        'delivery_records','delivery_lines','invoice_po_links','source_entity_links'] LOOP
        EXECUTE format(
            'CREATE POLICY %I ON %I FOR ALL USING (organization_id = nullif(current_setting(''app.organization_id'', true), '''')::uuid) WITH CHECK (organization_id = nullif(current_setting(''app.organization_id'', true), '''')::uuid)',
            table_name || '_tenant', table_name);
    END LOOP;
END $$;
