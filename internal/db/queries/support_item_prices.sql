-- Global NDIS Support Catalogue - NOT tenant-scoped (shared reference data).

-- name: ListSupportItemPrices :many
SELECT * FROM support_item_prices WHERE support_item_id = ? ORDER BY zone;

-- name: GetSupportItemPrice :one
SELECT * FROM support_item_prices WHERE support_item_id = ? AND zone = ?;

-- name: ResolveZonePrice :one
SELECT sip.*
FROM support_item_prices sip
JOIN support_items si ON sip.support_item_id = si.id
WHERE si.catalog_version_id = sqlc.arg(catalog_version_id)
  AND si.code = sqlc.arg(code)
  AND sip.zone = sqlc.arg(zone)
LIMIT 1;

-- name: CreateSupportItemPrice :one
INSERT INTO support_item_prices (support_item_id, zone, price_cap)
VALUES (?, ?, ?)
RETURNING *;

-- name: UpsertSupportItemPrice :exec
INSERT INTO support_item_prices (support_item_id, zone, price_cap)
VALUES (?, ?, ?)
ON CONFLICT (support_item_id, zone) DO UPDATE SET
    price_cap = excluded.price_cap;
