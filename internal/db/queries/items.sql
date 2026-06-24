-- Per-tenant price-list items (tenant-owned, scoped by tenant_id).

-- name: ListItems :many
SELECT * FROM items
WHERE tenant_id = sqlc.arg(tenant_id) AND price_list_version_id = sqlc.arg(version_id)
ORDER BY code;

-- name: SearchItems :many
-- All searchable fields (code, name, category, unit), tenant-scoped. sqlc.arg(q)
-- is the escaped LIKE pattern; pair with ESCAPE '\'.
SELECT * FROM items
WHERE tenant_id = sqlc.arg(tenant_id) AND price_list_version_id = sqlc.arg(version_id)
  AND ( (code     LIKE sqlc.arg(q) ESCAPE '\')
     OR (name     LIKE sqlc.arg(q) ESCAPE '\')
     OR (category LIKE sqlc.arg(q) ESCAPE '\')
     OR (unit     LIKE sqlc.arg(q) ESCAPE '\') )
ORDER BY code
LIMIT 50;

-- name: GetItem :one
SELECT * FROM items WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: GetItemIDByUUID :one
SELECT id FROM items WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(id);

-- name: GetItemByCode :one
SELECT * FROM items
WHERE tenant_id = sqlc.arg(tenant_id) AND price_list_version_id = sqlc.arg(version_id) AND code = sqlc.arg(code);

-- name: CreateItem :one
INSERT INTO items (
    tenant_id, id, price_list_version_id, code, name, unit, category, unit_price, taxable, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertItem :one
INSERT INTO items (
    tenant_id, id, price_list_version_id, code, name, unit, category, unit_price, taxable, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (price_list_version_id, code) DO UPDATE SET
    name = excluded.name,
    unit = excluded.unit,
    category = excluded.category,
    unit_price = excluded.unit_price,
    taxable = excluded.taxable,
    metadata = excluded.metadata
RETURNING *;

-- name: CountItems :one
SELECT COUNT(*) FROM items WHERE tenant_id = sqlc.arg(tenant_id) AND price_list_version_id = sqlc.arg(version_id);

-- name: DeleteItemsForVersion :exec
DELETE FROM items WHERE tenant_id = sqlc.arg(tenant_id) AND price_list_version_id = sqlc.arg(version_id);
