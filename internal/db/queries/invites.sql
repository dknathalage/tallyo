-- name: CreateInvite :one
INSERT INTO invites (id, tenant_id, token, email, role, created_by, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetInviteByToken :one
SELECT * FROM invites WHERE token = $1;

-- name: ListInvites :many
SELECT * FROM invites WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: MarkInviteAccepted :exec
UPDATE invites SET accepted_at = $1 WHERE token = $2;

-- name: DeleteInvite :exec
DELETE FROM invites WHERE tenant_id = $1 AND id = $2;

-- name: DeleteInviteByUUID :exec
DELETE FROM invites WHERE tenant_id = $1 AND id = $2;
