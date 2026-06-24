-- Per-tenant price list (tenant-owned, scoped by tenant_id).

-- name: ListPriceListVersions :many
SELECT * FROM price_list_versions WHERE tenant_id = sqlc.arg(tenant_id) ORDER BY effective_from DESC;

-- name: GetPriceListVersion :one
SELECT * FROM price_list_versions WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: GetPriceListVersionByUUID :one
SELECT * FROM price_list_versions WHERE tenant_id = sqlc.arg(tenant_id) AND uuid = sqlc.arg(uuid);

-- name: GetPriceListVersionIDByUUID :one
SELECT id FROM price_list_versions WHERE tenant_id = sqlc.arg(tenant_id) AND uuid = sqlc.arg(uuid);

-- name: ResolvePriceListVersionForDate :one
SELECT * FROM price_list_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND effective_from <= sqlc.arg(service_date)
  AND (effective_to IS NULL OR effective_to >= sqlc.arg(service_date))
ORDER BY effective_from DESC
LIMIT 1;

-- name: GetCurrentPriceListVersion :one
SELECT * FROM price_list_versions
WHERE tenant_id = sqlc.arg(tenant_id) AND effective_to IS NULL
ORDER BY effective_from DESC
LIMIT 1;

-- name: CreatePriceListVersion :one
INSERT INTO price_list_versions (tenant_id, uuid, label, effective_from, effective_to, source_filename, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ClosePriceListVersion :exec
UPDATE price_list_versions SET effective_to = sqlc.arg(effective_to)
WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: CloseOpenPriceListVersions :exec
-- Close every still-open (effective_to IS NULL) version for this tenant. Called
-- when a new version is ingested so date-windows never overlap and historical
-- service dates resolve to the version that was effective then.
UPDATE price_list_versions SET effective_to = sqlc.arg(effective_to)
WHERE tenant_id = sqlc.arg(tenant_id) AND effective_to IS NULL;

-- name: DeletePriceListVersion :exec
DELETE FROM price_list_versions WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);
