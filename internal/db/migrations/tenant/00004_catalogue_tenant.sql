-- +goose Up
-- The catalogue tables (price_list_versions, items) were created without a
-- tenant_id during the DB-per-tenant → single-DB collapse, leaving the price
-- list effectively global (every tenant shared one catalogue). Add tenant_id so
-- the catalogue is genuinely per-tenant, matching every other business table.
-- Backfill stamps existing rows with the sole tenant when exactly one exists
-- (the common clean-break case); multi-tenant installs reload catalogues after
-- this migration. New rows always carry tenant_id explicitly (set in app).
ALTER TABLE price_list_versions ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '';

UPDATE price_list_versions SET tenant_id = (SELECT id FROM tenants ORDER BY id LIMIT 1)
  WHERE (SELECT COUNT(*) FROM tenants) = 1;
UPDATE items SET tenant_id = (SELECT id FROM tenants ORDER BY id LIMIT 1)
  WHERE (SELECT COUNT(*) FROM tenants) = 1;

CREATE INDEX idx_plv_tenant_effective ON price_list_versions (tenant_id, effective_from, effective_to);
CREATE INDEX idx_items_tenant_version ON items (tenant_id, price_list_version_id);

-- +goose Down
DROP INDEX idx_items_tenant_version;
DROP INDEX idx_plv_tenant_effective;
ALTER TABLE items DROP COLUMN tenant_id;
ALTER TABLE price_list_versions DROP COLUMN tenant_id;
