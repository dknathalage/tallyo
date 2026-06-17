-- Global NDIS Support Catalogue - NOT tenant-scoped (shared reference data).

-- name: ListSupportItems :many
SELECT * FROM support_items WHERE catalog_version_id = ? ORDER BY code;

-- name: SearchSupportItems :many
SELECT * FROM support_items
WHERE catalog_version_id = ? AND ((code LIKE ? ESCAPE '\') OR (name LIKE ? ESCAPE '\'))
ORDER BY code;

-- name: GetSupportItem :one
SELECT * FROM support_items WHERE id = ?;

-- name: GetSupportItemByCode :one
SELECT * FROM support_items WHERE catalog_version_id = ? AND code = ?;

-- name: CreateSupportItem :one
INSERT INTO support_items (
    uuid, catalog_version_id, code, name, unit, support_category,
    registration_group, claim_type, gst_free, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpsertSupportItem :one
INSERT INTO support_items (
    uuid, catalog_version_id, code, name, unit, support_category,
    registration_group, claim_type, gst_free, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (catalog_version_id, code) DO UPDATE SET
    name = excluded.name,
    unit = excluded.unit,
    support_category = excluded.support_category,
    registration_group = excluded.registration_group,
    claim_type = excluded.claim_type,
    gst_free = excluded.gst_free,
    metadata = excluded.metadata
RETURNING *;

-- name: CountSupportItems :one
SELECT COUNT(*) FROM support_items WHERE catalog_version_id = ?;

-- name: DeleteSupportItemsForVersion :exec
DELETE FROM support_items WHERE catalog_version_id = ?;
