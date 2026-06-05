-- +goose Up
DROP TABLE column_mappings;

-- +goose Down
CREATE TABLE column_mappings (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid             TEXT NOT NULL UNIQUE,
    name             TEXT NOT NULL,
    entity_type      TEXT NOT NULL DEFAULT 'catalog',
    mapping          TEXT NOT NULL DEFAULT '{}',
    tier_mapping     TEXT DEFAULT '{}',
    metadata_mapping TEXT DEFAULT '[]',
    file_type        TEXT DEFAULT 'csv',
    sheet_name       TEXT DEFAULT '',
    header_row       INTEGER DEFAULT 1,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
