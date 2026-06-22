-- +goose Up
-- Per-tenant NDIS Support Catalogue (DB-per-tenant). Each tenant owns and
-- populates its own catalogue; there is no global seed. Line items pin to a
-- catalogue version/item by the stored UUID (TEXT), so these tables carry a
-- `uuid` column. Fresh clean-break schema — no data migration.

CREATE TABLE catalog_versions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL,           -- e.g. '2025-26 v1.1'
    effective_from  TEXT NOT NULL,           -- DATE
    effective_to    TEXT,                    -- DATE NULL (open-ended = current)
    source_filename TEXT DEFAULT '',
    created_at      TEXT NOT NULL
);
CREATE INDEX idx_catalog_versions_effective ON catalog_versions (effective_from, effective_to);

CREATE TABLE support_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    catalog_version_id INTEGER NOT NULL REFERENCES catalog_versions(id) ON DELETE CASCADE,
    code               TEXT NOT NULL,
    name               TEXT NOT NULL,
    unit               TEXT DEFAULT '',
    support_category   TEXT DEFAULT '' CHECK (support_category IN ('Core','CB','Capital','')),
    registration_group TEXT DEFAULT '',
    claim_type         TEXT DEFAULT '',
    taxable            INTEGER NOT NULL DEFAULT 1,
    metadata           TEXT DEFAULT '{}',
    UNIQUE (catalog_version_id, code)
);
CREATE INDEX idx_support_items_version ON support_items (catalog_version_id);
CREATE INDEX idx_support_items_code    ON support_items (code);

CREATE TABLE support_item_prices (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    support_item_id INTEGER NOT NULL REFERENCES support_items(id) ON DELETE CASCADE,
    zone            TEXT NOT NULL CHECK (zone IN ('national','remote','very_remote')),
    price_cap       REAL,                     -- NULL = quotable item (no fixed cap)
    UNIQUE (support_item_id, zone)
);
CREATE INDEX idx_support_item_prices_item ON support_item_prices (support_item_id);

-- +goose Down
DROP TABLE support_item_prices;
DROP TABLE support_items;
DROP TABLE catalog_versions;
