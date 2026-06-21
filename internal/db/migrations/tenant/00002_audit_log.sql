-- +goose Up
-- Per-tenant audit log. SAME column shape as control.db's audit_log (one shared
-- sqlc AuditLog model), but the cross-DB FKs are dropped: tenant_id and user_id
-- reference tables that live in control.db, so they are plain columns here
-- (tenant_id redundant — the file scopes the tenant; user_id non-authoritative).
--
-- This file is deliberately EXCLUDED from sqlc's schema input (sqlc reads the
-- control copy for the model); goose still applies it to every tenant DB.
-- IF NOT EXISTS so combined single-file mode (control audit_log already present)
-- is a no-op; a pure tenant DB creates it here.
CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,
    tenant_id   INTEGER,
    user_id     INTEGER,
    entity_type TEXT NOT NULL,
    entity_id   INTEGER,
    action      TEXT NOT NULL,
    changes     TEXT DEFAULT '{}',
    context     TEXT DEFAULT '',
    batch_id    TEXT,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_tenant  ON audit_log (tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_entity  ON audit_log (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_batch   ON audit_log (batch_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log (created_at);

-- +goose Down
DROP TABLE audit_log;
