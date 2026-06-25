-- Per-tenant catalogue (tenant-owned, scoped by tenant_id). One append-only
-- table with per-item copy-on-write versioning: is_current = 1 is the live row.

-- name: ListCatalogue :many
SELECT * FROM catalogue_items
WHERE tenant_id = ? AND is_current = 1
ORDER BY name;

-- name: SearchCatalogue :many
-- All searchable fields (code, name, category, unit), current rows only.
-- The LIKE pattern is escaped; pair with ESCAPE backslash.
SELECT * FROM catalogue_items
WHERE tenant_id = ? AND is_current = 1
  AND ( (code     LIKE ? ESCAPE '\')
     OR (name     LIKE ? ESCAPE '\')
     OR (category LIKE ? ESCAPE '\')
     OR (unit     LIKE ? ESCAPE '\') )
ORDER BY name
LIMIT 50;

-- name: GetCatalogueItem :one
-- A specific version row by id (any is_current). The validator reads a line
-- pinned version, which copy-on-write guarantees is frozen.
SELECT * FROM catalogue_items WHERE tenant_id = ? AND id = ?;

-- name: GetCurrentCatalogueByLogical :one
SELECT * FROM catalogue_items
WHERE tenant_id = ? AND logical_id = ? AND is_current = 1;

-- name: GetCurrentCatalogueByCode :one
-- The current row for a code (the import upsert key). Empty code never matches.
SELECT * FROM catalogue_items
WHERE tenant_id = ? AND is_current = 1 AND code = ? AND code <> '';

-- name: MaxCatalogueVersionForLogical :one
SELECT COALESCE(MAX(version), 0) FROM catalogue_items
WHERE tenant_id = ? AND logical_id = ?;

-- name: CreateCatalogueItem :one
INSERT INTO catalogue_items (
    id, logical_id, tenant_id, code, name, unit, category, unit_price, taxable,
    metadata, version, is_current, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCatalogueItemInPlace :one
UPDATE catalogue_items SET
    code = ?, name = ?, unit = ?, category = ?, unit_price = ?, taxable = ?,
    metadata = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: MarkCatalogueVersionStale :exec
UPDATE catalogue_items SET is_current = 0 WHERE tenant_id = ? AND id = ?;

-- name: TombstoneCatalogueLogical :exec
-- Delete flips every row of the logical_id out of current; referenced versions
-- linger so existing documents stay intact.
UPDATE catalogue_items SET is_current = 0
WHERE tenant_id = ? AND logical_id = ?;

-- name: GetCatalogueLogicalIDByUUID :one
-- Resolve a current version-row uuid to its logical_id (for bulk-delete; unknown
-- uuid returns no rows so the caller can 400).
SELECT logical_id FROM catalogue_items
WHERE tenant_id = ? AND id = ? AND is_current = 1;

-- name: LineItemReferencesCatalogue :one
SELECT EXISTS (SELECT 1 FROM line_items WHERE catalogue_item_id = ?);

-- name: EstimateLineReferencesCatalogue :one
SELECT EXISTS (SELECT 1 FROM estimate_line_items WHERE catalogue_item_id = ?);
