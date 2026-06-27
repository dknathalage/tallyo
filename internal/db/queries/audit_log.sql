-- name: ListAuditByTenant :many
-- Most recent audit rows for a tenant, newest first. Bounded to 50 rows for the
-- platform-admin tenant-detail trail (idx_audit_tenant + idx_audit_created back
-- the filter + order).
SELECT * FROM audit_log
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT 50;
