-- Per-tenant price-list item prices (tenant-owned).

-- name: ListItemPrices :many
SELECT * FROM item_prices WHERE item_id = ? ORDER BY zone;

-- name: GetItemPrice :one
SELECT * FROM item_prices WHERE item_id = ? AND zone = ?;

-- name: ResolveZonePrice :one
SELECT ip.*
FROM item_prices ip
JOIN items i ON ip.item_id = i.id
WHERE i.price_list_version_id = sqlc.arg(price_list_version_id)
  AND i.code = sqlc.arg(code)
  AND ip.zone = sqlc.arg(zone)
LIMIT 1;

-- name: CreateItemPrice :one
INSERT INTO item_prices (item_id, zone, price_cap)
VALUES (?, ?, ?)
RETURNING *;

-- name: UpsertItemPrice :exec
INSERT INTO item_prices (item_id, zone, price_cap)
VALUES (?, ?, ?)
ON CONFLICT (item_id, zone) DO UPDATE SET
    price_cap = excluded.price_cap;
