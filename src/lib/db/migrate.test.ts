/**
 * Migration smoke tests using better-sqlite3 in-memory database.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import Database from 'better-sqlite3';
import { runMigrations } from './migrate.js';

let db: Database.Database;

function setupDb(withSchema = true) {
	db = new Database(':memory:');
	db.pragma('foreign_keys = ON');
	if (withSchema) {
		runMigrations(db);
	}
}

function tableExists(name: string): boolean {
	const result = db
		.prepare(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`)
		.all(name) as { name: string }[];
	return result.length > 0;
}

function columnExists(table: string, column: string): boolean {
	const cols = db.prepare(`PRAGMA table_info(${table})`).all() as { name: string }[];
	return cols.some((c) => c.name === column);
}

describe('runMigrations', () => {
	beforeEach(() => {
		setupDb(false);
	});

	it('creates all base tables', () => {
		runMigrations(db);
		expect(tableExists('clients')).toBe(true);
		expect(tableExists('invoices')).toBe(true);
		expect(tableExists('line_items')).toBe(true);
		expect(tableExists('catalog_items')).toBe(true);
	});

	it('creates the payments table', () => {
		runMigrations(db);
		expect(tableExists('payments')).toBe(true);
	});

	it('adds currency_code to invoices', () => {
		runMigrations(db);
		expect(columnExists('invoices', 'currency_code')).toBe(true);
	});

	it('does not throw on a fresh schema database', () => {
		expect(() => runMigrations(db)).not.toThrow();
	});

	it('creates the estimates table', () => {
		runMigrations(db);
		expect(tableExists('estimates')).toBe(true);
	});

	it('creates the estimate_line_items table', () => {
		runMigrations(db);
		expect(tableExists('estimate_line_items')).toBe(true);
	});

	it('creates the tax_rates table', () => {
		runMigrations(db);
		expect(tableExists('tax_rates')).toBe(true);
	});

	it('creates the recurring_templates table', () => {
		runMigrations(db);
		expect(tableExists('recurring_templates')).toBe(true);
	});

	it('is idempotent - running twice does not throw', () => {
		runMigrations(db);
		expect(() => runMigrations(db)).not.toThrow();
	});
});
