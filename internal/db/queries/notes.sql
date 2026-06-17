-- Per-participant daily journal (tenant-scoped). See migration 00003_notes.sql.

-- name: ListParticipantNotes :many
SELECT * FROM notes
WHERE tenant_id = ? AND participant_id = ?
ORDER BY service_date, id;

-- name: ListParticipantNotesRange :many
SELECT * FROM notes
WHERE tenant_id = ? AND participant_id = ?
  AND service_date >= ? AND service_date <= ?
ORDER BY service_date, id;

-- name: GetNote :one
SELECT * FROM notes WHERE tenant_id = ? AND id = ?;

-- name: CreateNote :one
INSERT INTO notes (
    uuid, tenant_id, participant_id, service_date, body,
    transport_km, support_hours, author_user_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateNote :one
UPDATE notes SET
    service_date = ?, body = ?, transport_km = ?, support_hours = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: DeleteNote :exec
DELETE FROM notes WHERE tenant_id = ? AND id = ?;

-- name: MarkNoteBilled :exec
UPDATE notes SET billed_invoice_id = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?;
