-- name: ListRateTiers :many
SELECT * FROM rate_tiers ORDER BY sort_order, name;

-- name: GetRateTier :one
SELECT * FROM rate_tiers WHERE id = ?;

-- name: GetDefaultTier :one
SELECT * FROM rate_tiers ORDER BY sort_order, id LIMIT 1;

-- name: CountRateTiers :one
SELECT COUNT(*) FROM rate_tiers;

-- name: CreateRateTier :one
INSERT INTO rate_tiers (uuid, name, description, sort_order, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRateTier :one
UPDATE rate_tiers SET name = ?, description = ?, sort_order = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteRateTier :exec
DELETE FROM rate_tiers WHERE id = ?;
