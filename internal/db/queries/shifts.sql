-- Shifts: the delivered-support unit (tenant-scoped). See migration 00004_shifts.sql.

-- name: ListShifts :many
SELECT * FROM shifts WHERE tenant_id = ? ORDER BY service_date DESC, id DESC;

-- name: ListShiftsByParticipant :many
SELECT * FROM shifts
WHERE tenant_id = ? AND participant_id = ?
ORDER BY service_date, id;

-- name: ListShiftsByParticipantRange :many
SELECT * FROM shifts
WHERE tenant_id = ? AND participant_id = ?
  AND service_date >= ? AND service_date <= ?
ORDER BY service_date, id;

-- name: ListShiftsByStatus :many
SELECT * FROM shifts WHERE tenant_id = ? AND status = ? ORDER BY service_date, id;

-- name: ListScheduledShifts :many
SELECT * FROM shifts WHERE tenant_id = ? AND status = 'scheduled' ORDER BY service_date, id;

-- name: ListRecordedUnbilledByParticipant :many
SELECT * FROM shifts
WHERE tenant_id = ? AND participant_id = ? AND status = 'recorded' AND invoice_id IS NULL
ORDER BY service_date, id;

-- name: GetShift :one
SELECT * FROM shifts WHERE tenant_id = ? AND id = ?;

-- name: CreateShift :one
INSERT INTO shifts (
    uuid, tenant_id, participant_id, service_date, start_time, end_time,
    hours, km, measures, note, tags, status, author_user_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateShift :one
UPDATE shifts SET
    service_date = ?, start_time = ?, end_time = ?, hours = ?, km = ?,
    measures = ?, note = ?, tags = ?, status = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: UpdateShiftStatus :exec
UPDATE shifts SET status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: SetShiftInvoice :exec
UPDATE shifts SET invoice_id = ?, status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: SetStatusForInvoice :exec
UPDATE shifts SET status = ?, updated_at = ? WHERE tenant_id = ? AND invoice_id = ?;

-- name: ClearShiftsForInvoice :exec
UPDATE shifts SET invoice_id = NULL, status = 'recorded', updated_at = ?
WHERE tenant_id = ? AND invoice_id = ?;

-- name: DeleteShift :exec
DELETE FROM shifts WHERE tenant_id = ? AND id = ?;

-- name: ParticipantUnbilledAgg :many
SELECT participant_id, COUNT(*) AS cnt, MIN(service_date) AS from_date, MAX(service_date) AS to_date
FROM shifts
WHERE tenant_id = ? AND status = 'recorded' AND invoice_id IS NULL
GROUP BY participant_id;
