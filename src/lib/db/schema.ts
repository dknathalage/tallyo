export const CREATE_TABLES = `
CREATE TABLE IF NOT EXISTS clients (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT,
	name TEXT NOT NULL,
	email TEXT,
	phone TEXT,
	address TEXT,
	pricing_tier_id INTEGER REFERENCES rate_tiers(id) ON DELETE SET NULL,
	created_at TEXT DEFAULT (datetime('now')),
	updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS invoices (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT,
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
	uuid TEXT,
	invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
	description TEXT NOT NULL,
	quantity REAL NOT NULL DEFAULT 1,
	rate REAL NOT NULL DEFAULT 0,
	amount REAL NOT NULL DEFAULT 0,
	sort_order INTEGER DEFAULT 0,
	catalog_item_id INTEGER,
	rate_tier_id INTEGER
);

CREATE TABLE IF NOT EXISTS catalog_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT,
	name TEXT NOT NULL,
	rate REAL NOT NULL DEFAULT 0,
	unit TEXT DEFAULT '',
	category TEXT DEFAULT '',
	sku TEXT DEFAULT '',
	metadata TEXT DEFAULT '{}',
	created_at TEXT DEFAULT (datetime('now')),
	updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS rate_tiers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL UNIQUE,
	description TEXT DEFAULT '',
	sort_order INTEGER DEFAULT 0,
	created_at TEXT DEFAULT (datetime('now')),
	updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS catalog_item_rates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	catalog_item_id INTEGER NOT NULL REFERENCES catalog_items(id) ON DELETE CASCADE,
	rate_tier_id INTEGER NOT NULL REFERENCES rate_tiers(id) ON DELETE CASCADE,
	rate REAL NOT NULL DEFAULT 0,
	UNIQUE(catalog_item_id, rate_tier_id)
);

CREATE TABLE IF NOT EXISTS column_mappings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	entity_type TEXT NOT NULL DEFAULT 'catalog',
	mapping TEXT NOT NULL DEFAULT '{}',
	tier_mapping TEXT DEFAULT '{}',
	metadata_mapping TEXT DEFAULT '[]',
	file_type TEXT DEFAULT 'csv',
	sheet_name TEXT DEFAULT '',
	header_row INTEGER DEFAULT 1,
	created_at TEXT DEFAULT (datetime('now')),
	updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL UNIQUE,
	entity_type TEXT NOT NULL,
	entity_id INTEGER,
	action TEXT NOT NULL,
	changes TEXT DEFAULT '{}',
	context TEXT DEFAULT '',
	batch_id TEXT,
	created_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_batch ON audit_log(batch_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);
`;
