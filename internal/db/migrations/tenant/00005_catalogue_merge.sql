-- +goose Up
-- Merge custom_items + the price list (price_list_versions, items) into one
-- per-tenant catalogue with per-item copy-on-write versioning. Clean break: the
-- old catalogue tables are dropped (no data carried). line_items /
-- estimate_line_items keep their frozen snapshots; only the catalogue reference
-- collapses from three columns (item_id, custom_item_id, price_list_version_id)
-- into one catalogue_item_id. See docs/superpowers/specs/2026-06-25-catalogue-merge-design.md.

-- One append-only table. Each row IS a version of an item; rows sharing a
-- logical_id are that item's version history. is_current = 1 marks the live row.
CREATE TABLE catalogue_items (
    id          TEXT PRIMARY KEY,        -- uuidv7; the version row id (line items FK here)
    logical_id  TEXT NOT NULL,           -- stable identity across versions
    tenant_id   TEXT NOT NULL,           -- guard column (uuid)
    code        TEXT,                    -- optional; the import upsert key
    name        TEXT NOT NULL,
    unit        TEXT,
    category    TEXT,
    unit_price  REAL NOT NULL DEFAULT 0,
    taxable     INTEGER NOT NULL DEFAULT 0,
    metadata    TEXT NOT NULL DEFAULT '{}',
    version     INTEGER NOT NULL DEFAULT 1,
    is_current  INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX idx_catalogue_items_current ON catalogue_items (tenant_id, is_current);
CREATE INDEX idx_catalogue_items_logical ON catalogue_items (logical_id);
-- At most one current version per item; permits the delete-tombstone case where
-- every row of a logical_id is is_current = 0.
CREATE UNIQUE INDEX idx_catalogue_items_one_current ON catalogue_items (logical_id) WHERE is_current = 1;

-- Collapse the three catalogue refs on line_items into one catalogue_item_id.
-- Drop the item_id index first (DROP COLUMN forbids an indexed column), drop the
-- column's outgoing FK (to custom_items) by dropping the column, then add the new
-- FK to catalogue_items.
DROP INDEX idx_line_items_item;
ALTER TABLE line_items DROP COLUMN item_id;
ALTER TABLE line_items DROP COLUMN custom_item_id;
ALTER TABLE line_items DROP COLUMN price_list_version_id;
ALTER TABLE line_items ADD COLUMN catalogue_item_id TEXT REFERENCES catalogue_items(id) ON DELETE SET NULL;

ALTER TABLE estimate_line_items DROP COLUMN item_id;
ALTER TABLE estimate_line_items DROP COLUMN custom_item_id;
ALTER TABLE estimate_line_items DROP COLUMN price_list_version_id;
ALTER TABLE estimate_line_items ADD COLUMN catalogue_item_id TEXT REFERENCES catalogue_items(id) ON DELETE SET NULL;

-- Old catalogue tables gone (columns referencing custom_items already dropped).
DROP TABLE custom_items;
DROP TABLE items;
DROP TABLE price_list_versions;

-- +goose Down
-- Best-effort reverse (clean-break: down recreates the old tables empty).
DROP TABLE catalogue_items;

ALTER TABLE line_items DROP COLUMN catalogue_item_id;
ALTER TABLE line_items ADD COLUMN item_id TEXT DEFAULT '';
ALTER TABLE line_items ADD COLUMN custom_item_id TEXT;
ALTER TABLE line_items ADD COLUMN price_list_version_id TEXT DEFAULT '';
CREATE INDEX idx_line_items_item ON line_items(item_id);

ALTER TABLE estimate_line_items DROP COLUMN catalogue_item_id;
ALTER TABLE estimate_line_items ADD COLUMN item_id TEXT DEFAULT '';
ALTER TABLE estimate_line_items ADD COLUMN custom_item_id TEXT;
ALTER TABLE estimate_line_items ADD COLUMN price_list_version_id TEXT DEFAULT '';

CREATE TABLE custom_items (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    unit       TEXT DEFAULT '',
    taxable    INTEGER NOT NULL DEFAULT 1,
    metadata   TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_custom_items_tenant ON custom_items (tenant_id);

CREATE TABLE price_list_versions (
    id              TEXT PRIMARY KEY,
    label           TEXT NOT NULL,
    effective_from  TEXT NOT NULL,
    effective_to    TEXT,
    source_filename TEXT DEFAULT '',
    created_at      TEXT NOT NULL,
    tenant_id       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE items (
    id                    TEXT PRIMARY KEY,
    price_list_version_id TEXT NOT NULL REFERENCES price_list_versions(id) ON DELETE CASCADE,
    code                  TEXT NOT NULL,
    name                  TEXT NOT NULL,
    unit                  TEXT DEFAULT '',
    category              TEXT,
    unit_price            REAL,
    taxable               INTEGER NOT NULL DEFAULT 0,
    metadata              TEXT DEFAULT '{}',
    tenant_id             TEXT NOT NULL DEFAULT ''
);
