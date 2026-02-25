import { query, execute } from './connection.svelte.js';

const TABLES_WITH_UUID = ['clients', 'invoices', 'line_items', 'catalog_items'];

function tableHasColumn(table: string, column: string): boolean {
	const cols = query<{ name: string }>(`PRAGMA table_info(${table})`);
	return cols.some((c) => c.name === column);
}

function tableExists(table: string): boolean {
	const result = query<{ name: string }>(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
		[table]
	);
	return result.length > 0;
}

/** Migration 0: Add UUID columns to original tables */
function migration0_addUuids() {
	for (const table of TABLES_WITH_UUID) {
		if (!tableHasColumn(table, 'uuid')) {
			execute(`ALTER TABLE ${table} ADD COLUMN uuid TEXT`);
		}
		const rows = query<{ id: number }>(`SELECT id FROM ${table} WHERE uuid IS NULL`);
		for (const row of rows) {
			execute(`UPDATE ${table} SET uuid = ? WHERE id = ?`, [crypto.randomUUID(), row.id]);
		}
		execute(`CREATE UNIQUE INDEX IF NOT EXISTS idx_${table}_uuid ON ${table}(uuid)`);
	}
}

/** Migration 1: Add metadata column to catalog_items */
function migration1_catalogMetadata() {
	if (!tableHasColumn('catalog_items', 'metadata')) {
		execute(`ALTER TABLE catalog_items ADD COLUMN metadata TEXT DEFAULT '{}'`);
	}
}

/** Migration 2: Add pricing_tier_id to clients */
function migration2_clientTier() {
	if (!tableHasColumn('clients', 'pricing_tier_id')) {
		execute(`ALTER TABLE clients ADD COLUMN pricing_tier_id INTEGER REFERENCES rate_tiers(id) ON DELETE SET NULL`);
	}
}

/** Migration 3: Add catalog_item_id and rate_tier_id to line_items */
function migration3_lineItemRefs() {
	if (!tableHasColumn('line_items', 'catalog_item_id')) {
		execute(`ALTER TABLE line_items ADD COLUMN catalog_item_id INTEGER`);
	}
	if (!tableHasColumn('line_items', 'rate_tier_id')) {
		execute(`ALTER TABLE line_items ADD COLUMN rate_tier_id INTEGER`);
	}
}

/** Migration 4: Create default "Standard" tier and migrate existing rates */
function migration4_defaultTier() {
	if (!tableExists('rate_tiers')) return;

	const existing = query<{ id: number }>(`SELECT id FROM rate_tiers WHERE name = 'Standard'`);
	if (existing.length > 0) return;

	// Create default tier
	execute(
		`INSERT INTO rate_tiers (uuid, name, description, sort_order) VALUES (?, 'Standard', 'Default pricing tier', 0)`,
		[crypto.randomUUID()]
	);
	const tier = query<{ id: number }>(`SELECT id FROM rate_tiers WHERE name = 'Standard'`);
	if (tier.length === 0) return;
	const tierId = tier[0].id;

	// Migrate existing catalog item rates
	const items = query<{ id: number; rate: number }>(`SELECT id, rate FROM catalog_items`);
	for (const item of items) {
		const alreadyMigrated = query<{ id: number }>(
			`SELECT id FROM catalog_item_rates WHERE catalog_item_id = ? AND rate_tier_id = ?`,
			[item.id, tierId]
		);
		if (alreadyMigrated.length === 0) {
			execute(
				`INSERT INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate) VALUES (?, ?, ?)`,
				[item.id, tierId, item.rate]
			);
		}
	}
}

/** Migration 5: Add metadata/payer_id to clients, snapshot columns to invoices */
function migration5_metadataAndParties() {
	if (!tableHasColumn('clients', 'metadata')) {
		execute(`ALTER TABLE clients ADD COLUMN metadata TEXT DEFAULT '{}'`);
	}
	if (!tableHasColumn('clients', 'payer_id')) {
		execute(`ALTER TABLE clients ADD COLUMN payer_id INTEGER REFERENCES payers(id) ON DELETE SET NULL`);
	}
	execute(`CREATE INDEX IF NOT EXISTS idx_clients_payer ON clients(payer_id)`);
	if (!tableHasColumn('invoices', 'business_snapshot')) {
		execute(`ALTER TABLE invoices ADD COLUMN business_snapshot TEXT DEFAULT '{}'`);
	}
	if (!tableHasColumn('invoices', 'client_snapshot')) {
		execute(`ALTER TABLE invoices ADD COLUMN client_snapshot TEXT DEFAULT '{}'`);
	}
	if (!tableHasColumn('invoices', 'payer_snapshot')) {
		execute(`ALTER TABLE invoices ADD COLUMN payer_snapshot TEXT DEFAULT '{}'`);
	}
}

/** Run all migrations in order. Safe to call multiple times. */
export function runMigrations() {
	migration0_addUuids();
	migration1_catalogMetadata();
	migration2_clientTier();
	migration3_lineItemRefs();
	migration4_defaultTier();
	migration5_metadataAndParties();
}

// Keep backward-compatible export name
export const migrateAddUuids = runMigrations;
