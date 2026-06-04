-- +goose Up
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,
    entity_type TEXT NOT NULL,
    entity_id   INTEGER,
    action      TEXT NOT NULL,
    changes     TEXT DEFAULT '{}',
    context     TEXT DEFAULT '',
    batch_id    TEXT,
    created_at  TEXT
);
CREATE INDEX idx_audit_entity  ON audit_log (entity_type, entity_id);
CREATE INDEX idx_audit_batch   ON audit_log (batch_id);
CREATE INDEX idx_audit_created ON audit_log (created_at);

CREATE TABLE business_profile (
    id               INTEGER PRIMARY KEY,
    uuid             TEXT NOT NULL UNIQUE,
    name             TEXT NOT NULL DEFAULT '',
    email            TEXT DEFAULT '',
    phone            TEXT DEFAULT '',
    address          TEXT DEFAULT '',
    logo             TEXT DEFAULT '',
    metadata         TEXT DEFAULT '{}',
    default_currency TEXT DEFAULT 'USD',
    created_at       TEXT,
    updated_at       TEXT
);

-- +goose Down
DROP TABLE business_profile;
DROP TABLE audit_log;
