-- name: CreateInvite :one
INSERT INTO invites (token, email, role, created_by, expires_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInviteByToken :one
SELECT * FROM invites WHERE token = ?;

-- name: MarkInviteUsed :exec
UPDATE invites SET used_at = ? WHERE token = ?;
