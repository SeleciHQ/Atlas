-- Platform-level metadata. This table is intentionally not tenant-scoped:
-- it stores Atlas platform settings and generation markers, never customer data.
CREATE TABLE platform_metadata (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
