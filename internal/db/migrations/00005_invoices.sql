-- +goose Up
CREATE TABLE invoices (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid              TEXT NOT NULL UNIQUE,
    invoice_number    TEXT NOT NULL UNIQUE,
    client_id         INTEGER NOT NULL REFERENCES clients(id),
    date              TEXT NOT NULL,
    due_date          TEXT NOT NULL,
    payment_terms     TEXT DEFAULT 'custom',
    subtotal          REAL DEFAULT 0,
    tax_rate          REAL DEFAULT 0,
    tax_rate_id       INTEGER REFERENCES tax_rates(id) ON DELETE SET NULL,
    tax_amount        REAL DEFAULT 0,
    total             REAL DEFAULT 0,
    notes             TEXT DEFAULT '',
    status            TEXT DEFAULT 'draft',
    currency_code     TEXT DEFAULT 'USD',
    business_snapshot TEXT DEFAULT '{}',
    client_snapshot   TEXT DEFAULT '{}',
    payer_snapshot    TEXT DEFAULT '{}',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX idx_invoices_status ON invoices (status);
CREATE INDEX idx_invoices_client_id ON invoices (client_id);
CREATE INDEX idx_invoices_created_at ON invoices (created_at);

CREATE TABLE line_items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    invoice_id      INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description     TEXT NOT NULL,
    quantity        REAL NOT NULL DEFAULT 1,
    rate            REAL NOT NULL DEFAULT 0,
    amount          REAL NOT NULL DEFAULT 0,
    notes           TEXT DEFAULT '',
    sort_order      INTEGER DEFAULT 0,
    catalog_item_id INTEGER,
    rate_tier_id    INTEGER
);
CREATE INDEX idx_line_items_invoice_id ON line_items (invoice_id);

-- +goose Down
DROP TABLE line_items;
DROP TABLE invoices;
