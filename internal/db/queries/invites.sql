-- name: CreateInvite :one
INSERT INTO invites (id, tenant_id, token, email, role, created_by, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInviteByToken :one
SELECT * FROM invites WHERE token = ?;

-- name: ListInvites :many
SELECT * FROM invites WHERE tenant_id = ? ORDER BY created_at DESC;

-- name: MarkInviteAccepted :exec
UPDATE invites SET accepted_at = ? WHERE token = ?;

-- name: DeleteInvite :exec
DELETE FROM invites WHERE tenant_id = ? AND id = ?;

-- name: DeleteInviteByUUID :exec
DELETE FROM invites WHERE tenant_id = ? AND id = ?;
