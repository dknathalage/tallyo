/**
 * Server-side SQLite connection using better-sqlite3 + Drizzle ORM.
 * This file must only be imported from server-side code (+page.server.ts, API routes, etc.).
 * Never import directly in .svelte files.
 */
import { drizzle } from 'drizzle-orm/better-sqlite3';
import Database from 'better-sqlite3';
import { migrate } from 'drizzle-orm/better-sqlite3/migrator';
import * as schema from './drizzle-schema.js';
import { getDbPath } from '../data-dir.js';

let _sqlite: Database.Database | null = null;
let _db: ReturnType<typeof drizzle<typeof schema>> | null = null;
let _migrated = false;

function getSqlite(): Database.Database {
	if (_sqlite) return _sqlite;
	_sqlite = new Database(getDbPath());
	_sqlite.pragma('journal_mode = WAL');
	_sqlite.pragma('foreign_keys = ON');
	return _sqlite;
}

export function getDb() {
	if (_db) return _db;
	const sqlite = getSqlite();
	_db = drizzle(sqlite, { schema });
	return _db;
}

export type Database = ReturnType<typeof getDb>;

export function ensureMigrations(): void {
	if (_migrated) return;
	const db = getDb();
	migrate(db, { migrationsFolder: './drizzle' });
	_migrated = true;
}

export function healthCheck(): void {
	const sqlite = getSqlite();
	sqlite.prepare('SELECT 1').get();
}
