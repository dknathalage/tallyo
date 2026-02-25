import initSqlJs, { type Database } from 'sql.js';
import { base } from '$app/paths';
import { CREATE_TABLES } from './schema.js';
import { migrateAddUuids } from './migrate.js';
import { getIO } from '$lib/io/index.js';
import type { WebFileIO } from '$lib/io/web-io.js';

// Raw (non-proxied) reference to the Database instance.
// Svelte 5's $state deep proxy wraps objects in a Proxy, which
// interferes with sql.js's internal WASM operations and parameter binding.
let _instance: Database | null = null;

export const db = $state<{
	instance: Database | null;
	fileName: string;
	ioHandle: string | null;
	isOpen: boolean;
}>({
	instance: null,
	fileName: '',
	ioHandle: null,
	isOpen: false
});

/**
 * Check if there's a stored handle we can reconnect to.
 * If permission is already granted (or native), opens silently.
 * Returns the stored file name if a handle exists but needs user activation, or null.
 */
export async function tryRestore(): Promise<string | null> {
	const io = await getIO();
	const restored = await io.tryRestore();
	if (!restored) return null;

	// Try reading immediately — works when:
	// - FSAA permission is already granted (Chromium)
	// - IndexedDB storage (Safari/Firefox) — always works, no permission needed
	// - Native platform
	try {
		const opened = await openFromIO(io, restored.handle, restored.name);
		if (opened) return null; // opened silently
	} catch {
		// Fall through — permission may need user gesture (FSAA on Chromium)
	}

	// Permission needs user gesture — return the file name so UI can show a button
	return restored.name;
}

/** Reconnect to the stored handle (must be called from a user gesture on web for FSAA). */
export async function reconnect(): Promise<boolean> {
	const io = await getIO();

	// On web with File System Access API, we need to request permission via user gesture
	if ('requestPermission' in io) {
		const granted = await (io as WebFileIO).requestPermission();
		if (!granted) return false;
	}

	const restored = await io.tryRestore();
	if (!restored) return false;

	return await openFromIO(io, restored.handle, restored.name);
}

async function openFromIO(io: Awaited<ReturnType<typeof getIO>>, handle: string, name: string): Promise<boolean> {
	try {
		const data = await io.readFile(handle);
		const SQL = await initSql();
		const database = new SQL.Database(data);
		database.run('PRAGMA foreign_keys = ON;');
		database.run(CREATE_TABLES);
		_instance = database;
		migrateAddUuids();

		db.instance = database;
		db.fileName = name;
		db.ioHandle = handle;
		db.isOpen = true;
		return true;
	} catch {
		await io.clearStored();
		return false;
	}
}

async function initSql() {
	return await initSqlJs({
		locateFile: () => `${base}/sql-wasm.wasm`
	});
}

export async function initNew() {
	const io = await getIO();
	const SQL = await initSql();

	const handle = await io.pickNewFile('invoices.sqlite');
	const fileName = io.getFileName(handle);

	const database = new SQL.Database();
	database.run('PRAGMA foreign_keys = ON;');
	database.run(CREATE_TABLES);
	_instance = database;
	migrateAddUuids();

	db.instance = database;
	db.fileName = fileName;
	db.ioHandle = handle;
	db.isOpen = true;

	await save();
}

export async function openExisting() {
	const io = await getIO();
	const SQL = await initSql();

	const handle = await io.pickExistingFile();
	const data = await io.readFile(handle);
	const fileName = io.getFileName(handle);

	const database = new SQL.Database(data);
	database.run('PRAGMA foreign_keys = ON;');
	database.run(CREATE_TABLES);
	_instance = database;
	migrateAddUuids();

	db.instance = database;
	db.fileName = fileName;
	db.ioHandle = handle;
	db.isOpen = true;
}

export async function save() {
	if (!_instance) return;

	const io = await getIO();
	const data = _instance.export();

	if (db.ioHandle) {
		await io.writeFile(db.ioHandle, data);
	} else {
		// Fallback: trigger download
		const blob = new Blob([data as unknown as BlobPart], { type: 'application/x-sqlite3' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = db.fileName || 'invoices.sqlite';
		a.click();
		URL.revokeObjectURL(url);
	}
}

export async function close() {
	if (_instance) {
		_instance.close();
	}
	_instance = null;
	db.instance = null;
	db.fileName = '';
	db.ioHandle = null;
	db.isOpen = false;

	const io = await getIO();
	await io.clearStored();
}

export function runRaw(sql: string) {
	if (!_instance) throw new Error('Database not open');
	_instance.run(sql);
}

export function execute(sql: string, params?: unknown[]) {
	if (!_instance) throw new Error('Database not open');
	// Use prepare/bind/step instead of run() because the minified
	// sql.js build has a bug where Database.run() drops params
	// (Closure Compiler stripped the bind call from prepare()).
	const stmt = _instance.prepare(sql);
	try {
		if (params) stmt.bind(params as any);
		stmt.step();
	} finally {
		stmt.free();
	}
}

export function query<T = Record<string, unknown>>(sql: string, params?: unknown[]): T[] {
	if (!_instance) throw new Error('Database not open');
	const stmt = _instance.prepare(sql);
	if (params) stmt.bind(params as any);

	const results: T[] = [];
	while (stmt.step()) {
		results.push(stmt.getAsObject() as T);
	}
	stmt.free();
	return results;
}
