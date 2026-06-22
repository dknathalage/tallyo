-- Global NDIS Support Catalogue - NOT tenant-scoped (shared reference data).

-- name: ListCatalogVersions :many
SELECT * FROM catalog_versions ORDER BY effective_from DESC;

-- name: GetCatalogVersion :one
SELECT * FROM catalog_versions WHERE id = ?;

-- name: GetCatalogVersionByUUID :one
SELECT * FROM catalog_versions WHERE uuid = ?;

-- name: GetCatalogVersionIDByUUID :one
SELECT id FROM catalog_versions WHERE uuid = ?;

-- name: ResolveCatalogVersionForDate :one
SELECT * FROM catalog_versions
WHERE effective_from <= sqlc.arg(service_date)
  AND (effective_to IS NULL OR effective_to >= sqlc.arg(service_date))
ORDER BY effective_from DESC
LIMIT 1;

-- name: GetCurrentCatalogVersion :one
SELECT * FROM catalog_versions
WHERE effective_to IS NULL
ORDER BY effective_from DESC
LIMIT 1;

-- name: CreateCatalogVersion :one
INSERT INTO catalog_versions (uuid, label, effective_from, effective_to, source_filename, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CloseCatalogVersion :exec
UPDATE catalog_versions SET effective_to = ? WHERE id = ?;

-- name: CloseOpenCatalogVersions :exec
-- Close every still-open (effective_to IS NULL) version. Called when a new
-- version is ingested so date-windows never overlap and historical service dates
-- resolve to the version that was effective then.
UPDATE catalog_versions SET effective_to = ? WHERE effective_to IS NULL;

-- name: DeleteCatalogVersion :exec
DELETE FROM catalog_versions WHERE id = ?;
