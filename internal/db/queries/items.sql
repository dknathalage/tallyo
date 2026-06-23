-- Per-tenant price-list items (tenant-owned).

-- name: ListItems :many
SELECT * FROM items WHERE price_list_version_id = ? ORDER BY code;

-- name: SearchItems :many
SELECT * FROM items
WHERE price_list_version_id = ? AND ((code LIKE ? ESCAPE '\') OR (name LIKE ? ESCAPE '\'))
ORDER BY code;

-- name: GetItem :one
SELECT * FROM items WHERE id = ?;

-- name: GetItemIDByUUID :one
SELECT id FROM items WHERE uuid = ?;

-- name: GetItemByCode :one
SELECT * FROM items WHERE price_list_version_id = ? AND code = ?;

-- name: CreateItem :one
INSERT INTO items (
    uuid, price_list_version_id, code, name, unit, category, unit_price, taxable, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertItem :one
INSERT INTO items (
    uuid, price_list_version_id, code, name, unit, category, unit_price, taxable, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (price_list_version_id, code) DO UPDATE SET
    name = excluded.name,
    unit = excluded.unit,
    category = excluded.category,
    unit_price = excluded.unit_price,
    taxable = excluded.taxable,
    metadata = excluded.metadata
RETURNING *;

-- name: CountItems :one
SELECT COUNT(*) FROM items WHERE price_list_version_id = ?;

-- name: DeleteItemsForVersion :exec
DELETE FROM items WHERE price_list_version_id = ?;
