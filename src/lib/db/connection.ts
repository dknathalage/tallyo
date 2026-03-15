/**
 * Server-side SQLite connection using better-sqlite3.
 * This file must only be imported from server-side code (+page.server.ts, API routes, etc.).
 * Never import directly in .svelte files.
 */
import Database from 'better-sqlite3';
import { mkdirSync, existsSync } from 'fs';
import { join } from 'path';
import { homedir } from 'os';
import { runMigrations } from './migrate.js';

const DATA_DIR = process.env.DB_PATH
	? join(process.env.DB_PATH, '..')
	: join(homedir(), '.invoices');
const DB_PATH = process.env.DB_PATH ?? join(DATA_DIR, 'invoices.db');

let _db: Database.Database | null = null;

export function getDb(): Database.Database {
	if (_db) return _db;

	if (!existsSync(DATA_DIR)) {
		mkdirSync(DATA_DIR, { recursive: true });
	}

	_db = new Database(DB_PATH);
	_db.pragma('journal_mode = WAL');
	_db.pragma('foreign_keys = ON');

	runMigrations(_db);

	return _db;
}

/**
 * Execute a write statement (INSERT, UPDATE, DELETE).
 */
export function execute(sql: string, params?: unknown[]): void {
	const db = getDb();
	db.prepare(sql).run(...(params ?? []));
}

/**
 * Query rows and return typed results.
 */
export function query<T = Record<string, unknown>>(sql: string, params?: unknown[]): T[] {
	const db = getDb();
	return db.prepare(sql).all(...(params ?? [])) as T[];
}

/**
 * Execute raw SQL (e.g., DDL, PRAGMA).
 */
export function runRaw(sql: string): void {
	const db = getDb();
	db.exec(sql);
}

/**
 * No-op: kept for backward compatibility with code that called save().
 * Server-side SQLite writes are durable immediately — no manual save needed.
 */
export async function save(): Promise<void> {
	// No-op: better-sqlite3 writes are synchronous and durable.
}
