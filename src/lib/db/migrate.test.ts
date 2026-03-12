/**
 * Migration smoke tests using a real in-memory sql.js database.
 *
 * We replace the connection mock with actual sql.js so migrations run real SQL
 * and we can inspect the resulting schema.  The asm.js build avoids WASM path
 * issues in the Node/Vitest environment.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';

// We need to set up the real database BEFORE importing migrate.ts so the
// module-level mock factory can close over the `db` variable we'll create.
// We do this with a factory mock that delegates to a shared `impl` object.
const impl = {
	query: (_sql: string, _params?: unknown[]): Record<string, unknown>[] => [],
	execute: (_sql: string, _params?: unknown[]) => {}
};

vi.mock('./connection.svelte.js', () => ({
	query: (sql: string, params?: unknown[]) => impl.query(sql, params),
	execute: (sql: string, params?: unknown[]) => impl.execute(sql, params),
	save: vi.fn().mockResolvedValue(undefined),
	runRaw: vi.fn()
}));

// Dynamically import sql.js asm build (no WASM, works in Node out of the box).
// eslint-disable-next-line @typescript-eslint/no-require-imports
const initSqlJs = require('sql.js/dist/sql-asm.js');

import { CREATE_TABLES } from './schema.js';
import { runMigrations, migrateAddUuids } from './migrate.js';

// We keep a reference to the current in-memory database for each test.
let db: import('sql.js').Database;

/**
 * Set up a fresh schema database and wire the connection mock delegates to
 * the real sql.js instance.
 */
async function setupDb(withSchema = true) {
	const SQL = await initSqlJs();
	db = new SQL.Database();

	if (withSchema) {
		db.run(CREATE_TABLES);
	}

	impl.query = (sql: string, params?: unknown[]) => {
		const stmt = db.prepare(sql);
		try {
			stmt.bind((params as (string | number | null | Uint8Array)[]) ?? []);
			const rows: Record<string, unknown>[] = [];
			while (stmt.step()) {
				rows.push(stmt.getAsObject() as Record<string, unknown>);
			}
			return rows;
		} finally {
			stmt.free();
		}
	};

	impl.execute = (sql: string, params?: unknown[]) => {
		db.run(sql, (params as (string | number | null | Uint8Array)[]) ?? []);
	};
}

/** Check whether a table exists in the schema. */
function tableExists(name: string): boolean {
	const stmt = db.prepare(
		`SELECT COUNT(*) as n FROM sqlite_master WHERE type='table' AND name=?`
	);
	stmt.bind([name]);
	stmt.step();
	const row = stmt.getAsObject() as { n: number };
	stmt.free();
	return row.n > 0;
}

/** Check whether a column exists in a table. */
function columnExists(table: string, column: string): boolean {
	const stmt = db.prepare(`PRAGMA table_info(${table})`);
	const cols: string[] = [];
	while (stmt.step()) {
		const row = stmt.getAsObject() as { name: string };
		cols.push(row.name);
	}
	stmt.free();
	return cols.includes(column);
}

beforeEach(async () => {
	await setupDb();
});

// ---------------------------------------------------------------------------
// tableExists / tableHasColumn helpers (tested via schema + migrations)
// ---------------------------------------------------------------------------

describe('tableExists helper', () => {
	it('returns true for a table created by CREATE_TABLES', () => {
		expect(tableExists('invoices')).toBe(true);
		expect(tableExists('clients')).toBe(true);
		expect(tableExists('audit_log')).toBe(true);
	});

	it('returns false for a table that has not been created', () => {
		expect(tableExists('nonexistent_table_xyz')).toBe(false);
	});

	it('returns true for estimate tables after migration creates them', () => {
		// Before migration these do not exist in the base schema
		// (CREATE_TABLES does not include estimates or payments)
		expect(tableExists('payments')).toBe(false);
		runMigrations();
		expect(tableExists('payments')).toBe(true);
	});
});

describe('tableHasColumn helper', () => {
	it('detects columns that exist in the base schema', () => {
		expect(columnExists('invoices', 'id')).toBe(true);
		expect(columnExists('invoices', 'invoice_number')).toBe(true);
		expect(columnExists('clients', 'name')).toBe(true);
	});

	it('returns false for a column that does not exist in the table', () => {
		expect(columnExists('invoices', 'nonexistent_column_xyz')).toBe(false);
	});

	it('detects columns added by migrations', () => {
		// currency_code is added by migration6_multiCurrency
		expect(columnExists('invoices', 'currency_code')).toBe(false);
		runMigrations();
		expect(columnExists('invoices', 'currency_code')).toBe(true);
	});
});

// ---------------------------------------------------------------------------
// runMigrations / migrateAddUuids on a fresh schema
// ---------------------------------------------------------------------------

describe('runMigrations', () => {
	it('does not throw on a fresh schema database', () => {
		expect(() => runMigrations()).not.toThrow();
	});

	it('creates the estimates table', () => {
		runMigrations();
		expect(tableExists('estimates')).toBe(true);
	});

	it('creates the estimate_line_items table', () => {
		runMigrations();
		expect(tableExists('estimate_line_items')).toBe(true);
	});

	it('creates the payments table', () => {
		runMigrations();
		expect(tableExists('payments')).toBe(true);
	});

	it('creates the tax_rates table', () => {
		runMigrations();
		expect(tableExists('tax_rates')).toBe(true);
	});

	it('creates the recurring_templates table', () => {
		runMigrations();
		expect(tableExists('recurring_templates')).toBe(true);
	});

	it('adds currency_code column to invoices', () => {
		runMigrations();
		expect(columnExists('invoices', 'currency_code')).toBe(true);
	});

	it('adds payment_terms column to invoices', () => {
		runMigrations();
		expect(columnExists('invoices', 'payment_terms')).toBe(true);
	});

	it('adds uuid columns to tables that need them', () => {
		runMigrations();
		expect(columnExists('clients', 'uuid')).toBe(true);
		expect(columnExists('invoices', 'uuid')).toBe(true);
	});

	it('seeds a default GST tax rate', () => {
		runMigrations();
		const stmt = db.prepare(
			`SELECT name, rate, is_default FROM tax_rates WHERE is_default = 1`
		);
		stmt.step();
		const row = stmt.getAsObject() as { name: string; rate: number; is_default: number };
		stmt.free();
		expect(row.name).toBe('GST');
		expect(row.rate).toBe(10);
	});
});

describe('runMigrations idempotency', () => {
	it('is safe to run twice — no errors on second run', () => {
		runMigrations();
		expect(() => runMigrations()).not.toThrow();
	});

	it('does not duplicate the seeded GST tax rate on second run', () => {
		runMigrations();
		runMigrations();
		const stmt = db.prepare(`SELECT COUNT(*) as n FROM tax_rates WHERE name = 'GST'`);
		stmt.step();
		const row = stmt.getAsObject() as { n: number };
		stmt.free();
		expect(row.n).toBe(1);
	});

	it('does not duplicate the Standard rate tier on second run', () => {
		runMigrations();
		runMigrations();
		const stmt = db.prepare(`SELECT COUNT(*) as n FROM rate_tiers WHERE name = 'Standard'`);
		stmt.step();
		const row = stmt.getAsObject() as { n: number };
		stmt.free();
		expect(row.n).toBe(1);
	});
});

describe('migrateAddUuids alias', () => {
	it('is the same function as runMigrations', () => {
		expect(migrateAddUuids).toBe(runMigrations);
	});

	it('runs without throwing', () => {
		expect(() => migrateAddUuids()).not.toThrow();
	});
});
