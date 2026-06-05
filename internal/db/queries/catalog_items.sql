-- name: ListCatalogItems :many
SELECT * FROM catalog_items ORDER BY name;

-- name: SearchCatalogItems :many
SELECT * FROM catalog_items WHERE name LIKE ? OR sku LIKE ? OR category LIKE ? ORDER BY name;

-- name: GetCatalogItem :one
SELECT * FROM catalog_items WHERE id = ?;

-- name: ListCategories :many
SELECT DISTINCT category FROM catalog_items WHERE category <> '' ORDER BY category;

-- name: CreateCatalogItem :one
INSERT INTO catalog_items (uuid, name, rate, unit, category, sku, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateCatalogItem :one
UPDATE catalog_items SET name = ?, rate = ?, unit = ?, category = ?, sku = ?, metadata = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteCatalogItem :exec
DELETE FROM catalog_items WHERE id = ?;
