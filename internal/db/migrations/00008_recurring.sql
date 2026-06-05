-- +goose Up
CREATE TABLE recurring_templates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    client_id  INTEGER REFERENCES clients(id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    frequency  TEXT NOT NULL,
    next_due   TEXT NOT NULL,
    line_items TEXT NOT NULL DEFAULT '[]',
    tax_rate   REAL NOT NULL DEFAULT 0,
    notes      TEXT NOT NULL DEFAULT '',
    is_active  INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_recurring_client ON recurring_templates (client_id);
CREATE INDEX idx_recurring_next_due ON recurring_templates (next_due);

-- +goose Down
DROP TABLE recurring_templates;
