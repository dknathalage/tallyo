-- +goose Up
CREATE TABLE tax_rates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE clients (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    email           TEXT DEFAULT '',
    phone           TEXT DEFAULT '',
    address         TEXT DEFAULT '',
    pricing_tier_id INTEGER REFERENCES rate_tiers(id) ON DELETE SET NULL,
    metadata        TEXT DEFAULT '{}',
    payer_id        INTEGER REFERENCES payers(id) ON DELETE SET NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX idx_clients_payer ON clients (payer_id);

CREATE TABLE catalog_items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    unit       TEXT DEFAULT '',
    category   TEXT DEFAULT '',
    sku        TEXT DEFAULT '',
    metadata   TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE catalog_item_rates (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_item_id INTEGER NOT NULL REFERENCES catalog_items(id) ON DELETE CASCADE,
    rate_tier_id    INTEGER NOT NULL REFERENCES rate_tiers(id) ON DELETE CASCADE,
    rate            REAL NOT NULL DEFAULT 0,
    UNIQUE (catalog_item_id, rate_tier_id)
);

-- +goose Down
DROP TABLE catalog_item_rates;
DROP TABLE catalog_items;
DROP TABLE clients;
DROP TABLE tax_rates;
