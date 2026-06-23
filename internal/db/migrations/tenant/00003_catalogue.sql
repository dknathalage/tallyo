-- +goose Up
-- Per-tenant price list (DB-per-tenant). Each tenant owns and populates its own
-- price list; there is no global seed. Line items pin to a price-list
-- version/item by the stored UUID (TEXT), so these tables carry a `uuid` column.
-- Fresh clean-break schema — no data migration.

CREATE TABLE price_list_versions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL,           -- e.g. '2025-26 v1.1'
    effective_from  TEXT NOT NULL,           -- DATE
    effective_to    TEXT,                    -- DATE NULL (open-ended = current)
    source_filename TEXT DEFAULT '',
    created_at      TEXT NOT NULL
);
CREATE INDEX idx_price_list_versions_effective ON price_list_versions (effective_from, effective_to);

CREATE TABLE items (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid                  TEXT NOT NULL UNIQUE,
    price_list_version_id INTEGER NOT NULL REFERENCES price_list_versions(id) ON DELETE CASCADE,
    code                  TEXT NOT NULL,
    name                  TEXT NOT NULL,
    unit                  TEXT DEFAULT '',
    category              TEXT,                       -- generic grouping (was NDIS support_category/registration_group/claim_type)
    unit_price            REAL,                       -- generic per-unit price (NULL = none/use zone caps)
    taxable               INTEGER NOT NULL DEFAULT 0, -- ingest always sets this explicitly
    metadata              TEXT DEFAULT '{}',
    UNIQUE (price_list_version_id, code)
);
CREATE INDEX idx_items_version ON items (price_list_version_id);
CREATE INDEX idx_items_code    ON items (code);

CREATE TABLE item_prices (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id         INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    zone            TEXT NOT NULL CHECK (zone IN ('national','remote','very_remote')),
    price_cap       REAL,                     -- NULL = quotable item (no fixed cap)
    UNIQUE (item_id, zone)
);
CREATE INDEX idx_item_prices_item ON item_prices (item_id);

-- +goose Down
DROP TABLE item_prices;
DROP TABLE items;
DROP TABLE price_list_versions;
