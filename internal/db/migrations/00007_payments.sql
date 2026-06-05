-- +goose Up
CREATE TABLE payments (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid         TEXT NOT NULL UNIQUE,
    invoice_id   INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    amount       REAL NOT NULL,
    payment_date TEXT NOT NULL,
    method       TEXT DEFAULT '',
    notes        TEXT DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX idx_payments_invoice_id ON payments (invoice_id);

-- +goose Down
DROP TABLE payments;
