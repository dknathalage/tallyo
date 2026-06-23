-- Per-tenant price list (tenant-owned).

-- name: ListPriceListVersions :many
SELECT * FROM price_list_versions ORDER BY effective_from DESC;

-- name: GetPriceListVersion :one
SELECT * FROM price_list_versions WHERE id = ?;

-- name: GetPriceListVersionByUUID :one
SELECT * FROM price_list_versions WHERE uuid = ?;

-- name: GetPriceListVersionIDByUUID :one
SELECT id FROM price_list_versions WHERE uuid = ?;

-- name: ResolvePriceListVersionForDate :one
SELECT * FROM price_list_versions
WHERE effective_from <= sqlc.arg(service_date)
  AND (effective_to IS NULL OR effective_to >= sqlc.arg(service_date))
ORDER BY effective_from DESC
LIMIT 1;

-- name: GetCurrentPriceListVersion :one
SELECT * FROM price_list_versions
WHERE effective_to IS NULL
ORDER BY effective_from DESC
LIMIT 1;

-- name: CreatePriceListVersion :one
INSERT INTO price_list_versions (uuid, label, effective_from, effective_to, source_filename, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ClosePriceListVersion :exec
UPDATE price_list_versions SET effective_to = ? WHERE id = ?;

-- name: CloseOpenPriceListVersions :exec
-- Close every still-open (effective_to IS NULL) version. Called when a new
-- version is ingested so date-windows never overlap and historical service dates
-- resolve to the version that was effective then.
UPDATE price_list_versions SET effective_to = ? WHERE effective_to IS NULL;

-- name: DeletePriceListVersion :exec
DELETE FROM price_list_versions WHERE id = ?;
