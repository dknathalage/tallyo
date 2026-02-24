export const CREATE_TABLES = `
CREATE TABLE IF NOT EXISTS clients (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT,
	phone TEXT,
	address TEXT,
	created_at TEXT DEFAULT (datetime('now')),
	updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS invoices (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	invoice_number TEXT NOT NULL UNIQUE,
	client_id INTEGER NOT NULL REFERENCES clients(id),
	date TEXT NOT NULL,
	due_date TEXT NOT NULL,
	subtotal REAL DEFAULT 0,
	tax_rate REAL DEFAULT 0,
	tax_amount REAL DEFAULT 0,
	total REAL DEFAULT 0,
	notes TEXT,
	status TEXT DEFAULT 'draft',
	created_at TEXT DEFAULT (datetime('now')),
	updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS line_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
	description TEXT NOT NULL,
	quantity REAL NOT NULL DEFAULT 1,
	rate REAL NOT NULL DEFAULT 0,
	amount REAL NOT NULL DEFAULT 0,
	sort_order INTEGER DEFAULT 0
);
`;
