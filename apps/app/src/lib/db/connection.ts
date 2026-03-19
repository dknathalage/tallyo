/**
 * Server-side PostgreSQL connection using pg + Drizzle ORM.
 * This file must only be imported from server-side code (+page.server.ts, API routes, etc.).
 * Never import directly in .svelte files.
 */
import { drizzle } from 'drizzle-orm/node-postgres';
import pg from 'pg';
import { migrate } from 'drizzle-orm/node-postgres/migrator';
import * as schema from './drizzle-schema.js';

const DATABASE_URL = process.env.DATABASE_URL ?? 'postgresql://localhost:5432/tallyo';

let _pool: pg.Pool | null = null;
let _db: ReturnType<typeof drizzle<typeof schema>> | null = null;
let _migrated = false;

function getPool(): pg.Pool {
	if (_pool) return _pool;
	_pool = new pg.Pool({ connectionString: DATABASE_URL });
	return _pool;
}

export function getDb() {
	if (_db) return _db;
	const pool = getPool();
	_db = drizzle(pool, { schema });
	return _db;
}

export type Database = ReturnType<typeof getDb>;

export async function ensureMigrations(): Promise<void> {
	if (_migrated) return;
	const db = getDb();
	await migrate(db, { migrationsFolder: './drizzle' });
	_migrated = true;
}

export async function healthCheck(): Promise<void> {
	const pool = getPool();
	const client = await pool.connect();
	try {
		await client.query('SELECT 1');
	} finally {
		client.release();
	}
}
