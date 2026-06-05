-- name: ListColumnMappings :many
SELECT * FROM column_mappings ORDER BY name;

-- name: ListColumnMappingsByEntity :many
SELECT * FROM column_mappings WHERE entity_type = ? ORDER BY name;

-- name: GetColumnMapping :one
SELECT * FROM column_mappings WHERE id = ?;

-- name: CreateColumnMapping :one
INSERT INTO column_mappings (uuid, name, entity_type, mapping, tier_mapping, metadata_mapping, file_type, sheet_name, header_row, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateColumnMapping :one
UPDATE column_mappings SET name = ?, entity_type = ?, mapping = ?, tier_mapping = ?, metadata_mapping = ?, file_type = ?, sheet_name = ?, header_row = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteColumnMapping :exec
DELETE FROM column_mappings WHERE id = ?;
