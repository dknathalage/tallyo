import type Database from 'better-sqlite3';
import { CREATE_TABLES } from './schema.js';

function tableHasColumn(db: Database.Database, table: string, column: string): boolean {
	const cols = db.prepare(`PRAGMA table_info(${table})`).all() as { name: string }[];
	return cols.some((c) => c.name === column);
}

function tableExists(db: Database.Database, table: string): boolean {
	const result = db
		.prepare(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`)
		.all(table) as { name: string }[];
	return result.length > 0;
}

/** Migration 0: Add UUID columns to original tables */
function migration0_addUuids(db: Database.Database) {
	const TABLES_WITH_UUID = ['clients', 'invoices', 'line_items', 'catalog_items'];
	for (const table of TABLES_WITH_UUID) {
		if (!tableHasColumn(db, table, 'uuid')) {
			db.exec(`ALTER TABLE ${table} ADD COLUMN uuid TEXT`);
		}
		const rows = db.prepare(`SELECT id FROM ${table} WHERE uuid IS NULL`).all() as { id: number }[];
		for (const row of rows) {
			db.prepare(`UPDATE ${table} SET uuid = ? WHERE id = ?`).run(crypto.randomUUID(), row.id);
		}
		db.exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_${table}_uuid ON ${table}(uuid)`);
	}
}

/** Migration 1: Add metadata column to catalog_items */
function migration1_catalogMetadata(db: Database.Database) {
	if (!tableHasColumn(db, 'catalog_items', 'metadata')) {
		db.exec(`ALTER TABLE catalog_items ADD COLUMN metadata TEXT DEFAULT '{}'`);
	}
}

/** Migration 2: Add pricing_tier_id to clients */
function migration2_clientTier(db: Database.Database) {
	if (!tableHasColumn(db, 'clients', 'pricing_tier_id')) {
		db.exec(
			`ALTER TABLE clients ADD COLUMN pricing_tier_id INTEGER REFERENCES rate_tiers(id) ON DELETE SET NULL`
		);
	}
}

/** Migration 3: Add catalog_item_id and rate_tier_id to line_items */
function migration3_lineItemRefs(db: Database.Database) {
	if (!tableHasColumn(db, 'line_items', 'catalog_item_id')) {
		db.exec(`ALTER TABLE line_items ADD COLUMN catalog_item_id INTEGER`);
	}
	if (!tableHasColumn(db, 'line_items', 'rate_tier_id')) {
		db.exec(`ALTER TABLE line_items ADD COLUMN rate_tier_id INTEGER`);
	}
}

/** Migration 4: Create default "Standard" tier and migrate existing rates */
function migration4_defaultTier(db: Database.Database) {
	if (!tableExists(db, 'rate_tiers')) return;

	const existing = db
		.prepare(`SELECT id FROM rate_tiers WHERE name = 'Standard'`)
		.all() as { id: number }[];
	if (existing.length > 0) return;

	db.prepare(
		`INSERT INTO rate_tiers (uuid, name, description, sort_order) VALUES (?, 'Standard', 'Default pricing tier', 0)`
	).run(crypto.randomUUID());
	const tier = db
		.prepare(`SELECT id FROM rate_tiers WHERE name = 'Standard'`)
		.all() as { id: number }[];
	if (tier.length === 0) return;
	const tierId = tier[0].id;

	const items = db
		.prepare(`SELECT id, rate FROM catalog_items`)
		.all() as { id: number; rate: number }[];
	for (const item of items) {
		const alreadyMigrated = db
			.prepare(
				`SELECT id FROM catalog_item_rates WHERE catalog_item_id = ? AND rate_tier_id = ?`
			)
			.all(item.id, tierId) as { id: number }[];
		if (alreadyMigrated.length === 0) {
			db.prepare(
				`INSERT INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate) VALUES (?, ?, ?)`
			).run(item.id, tierId, item.rate);
		}
	}
}

/** Migration 5: Add metadata/payer_id to clients, snapshot columns to invoices */
function migration5_metadataAndParties(db: Database.Database) {
	if (!tableHasColumn(db, 'clients', 'metadata')) {
		db.exec(`ALTER TABLE clients ADD COLUMN metadata TEXT DEFAULT '{}'`);
	}
	if (!tableHasColumn(db, 'clients', 'payer_id')) {
		db.exec(
			`ALTER TABLE clients ADD COLUMN payer_id INTEGER REFERENCES payers(id) ON DELETE SET NULL`
		);
	}
	db.exec(`CREATE INDEX IF NOT EXISTS idx_clients_payer ON clients(payer_id)`);
	if (!tableHasColumn(db, 'invoices', 'business_snapshot')) {
		db.exec(`ALTER TABLE invoices ADD COLUMN business_snapshot TEXT DEFAULT '{}'`);
	}
	if (!tableHasColumn(db, 'invoices', 'client_snapshot')) {
		db.exec(`ALTER TABLE invoices ADD COLUMN client_snapshot TEXT DEFAULT '{}'`);
	}
	if (!tableHasColumn(db, 'invoices', 'payer_snapshot')) {
		db.exec(`ALTER TABLE invoices ADD COLUMN payer_snapshot TEXT DEFAULT '{}'`);
	}
}

/** Migration 6: Add multi-currency support */
function migration6_multiCurrency(db: Database.Database) {
	if (!tableHasColumn(db, 'invoices', 'currency_code')) {
		db.exec(`ALTER TABLE invoices ADD COLUMN currency_code TEXT DEFAULT 'USD'`);
	}
	if (!tableHasColumn(db, 'business_profile', 'default_currency')) {
		db.exec(`ALTER TABLE business_profile ADD COLUMN default_currency TEXT DEFAULT 'USD'`);
	}
}

/** Migration 7: Create estimates and estimate_line_items tables */
function migration7_estimates(db: Database.Database) {
	if (!tableExists(db, 'estimates')) {
		db.exec(`CREATE TABLE IF NOT EXISTS estimates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT UNIQUE,
			estimate_number TEXT UNIQUE NOT NULL,
			client_id INTEGER REFERENCES clients(id),
			date TEXT NOT NULL,
			valid_until TEXT NOT NULL,
			subtotal REAL DEFAULT 0,
			tax_rate REAL DEFAULT 0,
			tax_amount REAL DEFAULT 0,
			total REAL DEFAULT 0,
			notes TEXT DEFAULT '',
			status TEXT DEFAULT 'draft',
			currency_code TEXT DEFAULT 'USD',
			converted_invoice_id INTEGER,
			business_snapshot TEXT DEFAULT '{}',
			client_snapshot TEXT DEFAULT '{}',
			payer_snapshot TEXT DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`);
	}
	if (!tableExists(db, 'estimate_line_items')) {
		db.exec(`CREATE TABLE IF NOT EXISTS estimate_line_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT UNIQUE,
			estimate_id INTEGER REFERENCES estimates(id) ON DELETE CASCADE,
			description TEXT NOT NULL,
			quantity REAL DEFAULT 1,
			rate REAL DEFAULT 0,
			amount REAL DEFAULT 0,
			notes TEXT DEFAULT '',
			sort_order INTEGER DEFAULT 0,
			catalog_item_id INTEGER,
			rate_tier_id INTEGER
		)`);
	}
}

/** Migration 8: Add payment_terms to invoices */
function migration8_paymentTerms(db: Database.Database) {
	if (!tableHasColumn(db, 'invoices', 'payment_terms')) {
		db.exec(`ALTER TABLE invoices ADD COLUMN payment_terms TEXT DEFAULT 'custom'`);
	}
}

/** Migration 9: Add tax_rates table */
function migration9_taxRates(db: Database.Database) {
	if (!tableExists(db, 'tax_rates')) {
		db.exec(`CREATE TABLE IF NOT EXISTS tax_rates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			rate REAL NOT NULL DEFAULT 0,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`);
		db.prepare(`INSERT INTO tax_rates (uuid, name, rate, is_default) VALUES (?, 'GST', 10, 1)`).run(
			crypto.randomUUID()
		);
	}
	if (!tableHasColumn(db, 'invoices', 'tax_rate_id')) {
		db.exec(
			`ALTER TABLE invoices ADD COLUMN tax_rate_id INTEGER REFERENCES tax_rates(id) ON DELETE SET NULL`
		);
	}
	if (!tableExists(db, 'estimates')) return;
	if (!tableHasColumn(db, 'estimates', 'tax_rate_id')) {
		db.exec(
			`ALTER TABLE estimates ADD COLUMN tax_rate_id INTEGER REFERENCES tax_rates(id) ON DELETE SET NULL`
		);
	}
}

/** Migration 10: Add payments table */
function migration10_payments(db: Database.Database) {
	if (!tableExists(db, 'payments')) {
		db.exec(`CREATE TABLE IF NOT EXISTS payments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE,
			invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
			amount REAL NOT NULL,
			payment_date TEXT NOT NULL,
			method TEXT DEFAULT '',
			notes TEXT DEFAULT '',
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`);
	}
}

/** Migration 11: Create recurring_templates table */
function migration11_recurringTemplates(db: Database.Database) {
	if (!tableExists(db, 'recurring_templates')) {
		db.exec(`CREATE TABLE IF NOT EXISTS recurring_templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE,
			client_id INTEGER REFERENCES clients(id) ON DELETE SET NULL,
			name TEXT NOT NULL,
			frequency TEXT NOT NULL CHECK (frequency IN ('weekly', 'monthly', 'quarterly')),
			next_due TEXT NOT NULL,
			line_items TEXT NOT NULL DEFAULT '[]',
			tax_rate REAL NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`);
		db.exec(`CREATE INDEX IF NOT EXISTS idx_recurring_client ON recurring_templates(client_id)`);
		db.exec(`CREATE INDEX IF NOT EXISTS idx_recurring_next_due ON recurring_templates(next_due)`);
	}
}

/** Migration 12: Add performance indexes on frequently-queried columns */
function migration12_performanceIndexes(db: Database.Database) {
	db.exec(`CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status)`);
	db.exec(`CREATE INDEX IF NOT EXISTS idx_invoices_client_id ON invoices(client_id)`);
	db.exec(`CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices(created_at DESC)`);
	if (tableExists(db, 'estimates')) {
		db.exec(`CREATE INDEX IF NOT EXISTS idx_estimates_status ON estimates(status)`);
		db.exec(`CREATE INDEX IF NOT EXISTS idx_estimates_client_id ON estimates(client_id)`);
	}
	db.exec(`CREATE INDEX IF NOT EXISTS idx_line_items_invoice_id ON line_items(invoice_id)`);
	if (tableExists(db, 'payments')) {
		db.exec(`CREATE INDEX IF NOT EXISTS idx_payments_invoice_id ON payments(invoice_id)`);
	}
	db.exec(`CREATE INDEX IF NOT EXISTS idx_audit_log_entity ON audit_log(entity_type, entity_id)`);
}

/** Migration 13: Create ai_chat_sessions table */
function migration13_aiChatSessions(db: Database.Database) {
	if (!tableExists(db, 'ai_chat_sessions')) {
		db.exec(`CREATE TABLE IF NOT EXISTS ai_chat_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE DEFAULT (lower(hex(randomblob(16)))),
			title TEXT NOT NULL DEFAULT 'New Chat',
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`);
		db.exec(`CREATE INDEX IF NOT EXISTS idx_ai_sessions_created ON ai_chat_sessions(created_at DESC)`);
	}
}

/** Migration 14: Create ai_chat_messages table */
function migration14_aiChatMessages(db: Database.Database) {
	if (!tableExists(db, 'ai_chat_messages')) {
		db.exec(`CREATE TABLE IF NOT EXISTS ai_chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT NOT NULL UNIQUE DEFAULT (lower(hex(randomblob(16)))),
			session_id INTEGER NOT NULL REFERENCES ai_chat_sessions(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content TEXT NOT NULL DEFAULT '',
			tool_calls TEXT DEFAULT NULL,
			tool_results TEXT DEFAULT NULL,
			is_streaming INTEGER NOT NULL DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now'))
		)`);
		db.exec(`CREATE INDEX IF NOT EXISTS idx_ai_messages_session ON ai_chat_messages(session_id, created_at)`);
	}
}

/** Run all migrations in order. Safe to call multiple times. */
export function runMigrations(db: Database.Database): void {
	db.exec(CREATE_TABLES);
	migration0_addUuids(db);
	migration1_catalogMetadata(db);
	migration2_clientTier(db);
	migration3_lineItemRefs(db);
	migration4_defaultTier(db);
	migration5_metadataAndParties(db);
	migration6_multiCurrency(db);
	migration7_estimates(db);
	migration8_paymentTerms(db);
	migration9_taxRates(db);
	migration10_payments(db);
	migration11_recurringTemplates(db);
	migration12_performanceIndexes(db);
	migration13_aiChatSessions(db);
	migration14_aiChatMessages(db);
}

// Backward-compatible alias
export const migrateAddUuids = runMigrations;
