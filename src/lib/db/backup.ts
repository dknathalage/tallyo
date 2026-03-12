/**
 * Database backup and restore utilities.
 *
 * Uses sql.js `db.export()` to serialise the in-memory SQLite database to a
 * Uint8Array, then either downloads it (export) or restores it into the
 * current connection (import).
 *
 * Works fully offline — no network required.
 */

import initSqlJs from 'sql.js';
import { base } from '$app/paths';
import { db } from '$lib/db/connection.svelte';
import { CREATE_TABLES } from '$lib/db/schema.js';
import { migrateAddUuids } from '$lib/db/migrate.js';

// Internal reference to the raw Database instance (same as connection.svelte.ts uses)
// We access it via the exported db state's instance.
// NOTE: sql.js export() is called on the live _instance, not the Svelte proxy.
// We obtain the raw instance through the connection module's save() path instead.

/**
 * Export the current database to a `.sqlite` file and trigger a browser download.
 * Returns the generated Blob (useful for testing / alternative delivery).
 */
export function exportDatabase(): Blob {
	const instance = db.instance;
	if (!instance) throw new Error('No database is open');

	const data = instance.export();
	const blob = new Blob([data as unknown as BlobPart], { type: 'application/x-sqlite3' });

	const date = new Date().toISOString().slice(0, 10); // YYYY-MM-DD
	const filename = `invoices-backup-${date}.sqlite`;

	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	URL.revokeObjectURL(url);

	return blob;
}

/**
 * Restore the database from an uploaded `.sqlite` file.
 *
 * ⚠️  Destructive — overwrites all current data.
 *
 * Steps:
 * 1. Read the file as ArrayBuffer.
 * 2. Open a new sql.js Database from the bytes.
 * 3. Persist to the current storage backend (FileSystemFileHandle or IndexedDB).
 * 4. Reload the page so the app re-initialises cleanly.
 */
export async function importDatabase(file: File): Promise<void> {
	const buffer = await file.arrayBuffer();
	const bytes = new Uint8Array(buffer);

	// Validate: SQLite files start with the magic header string
	const magic = new TextDecoder().decode(bytes.slice(0, 16));
	if (!magic.startsWith('SQLite format 3')) {
		throw new Error('The selected file does not appear to be a valid SQLite database.');
	}

	const SQL = await initSqlJs({
		locateFile: () => `${base}/sql-wasm.wasm`
	});

	// Open the uploaded DB to verify it can be parsed
	const testDb = new SQL.Database(bytes);
	testDb.close();

	// Persist using the current storage backend
	if (db.fileHandle) {
		// Chromium: write directly to the file via File System Access API
		const blob = new Blob([bytes as unknown as BlobPart], { type: 'application/x-sqlite3' });
		const writable = await (db.fileHandle as FileSystemFileHandle & {
			createWritable(): Promise<FileSystemWritableFileStream>;
		}).createWritable();
		await writable.write(blob);
		await writable.close();
	} else if (db.idbKey) {
		// Safari/Firefox: persist in IndexedDB
		const IDB_NAME = 'invoice-manager';
		const IDB_DB_STORE = 'db-storage';
		const IDB_META_KEY = '__meta__';
		const idbKey = db.idbKey;
		const fileName = db.fileName;

		const idb = await new Promise<IDBDatabase>((resolve, reject) => {
			const req = indexedDB.open(IDB_NAME, 2);
			req.onsuccess = () => resolve(req.result);
			req.onerror = () => reject(req.error);
		});

		await new Promise<void>((resolve, reject) => {
			const tx = idb.transaction(IDB_DB_STORE, 'readwrite');
			tx.objectStore(IDB_DB_STORE).put(bytes, idbKey);
			tx.objectStore(IDB_DB_STORE).put({ key: idbKey, name: fileName }, IDB_META_KEY);
			tx.oncomplete = () => resolve();
			tx.onerror = () => reject(tx.error);
		});

		idb.close();
	} else {
		throw new Error('No active database connection to restore into. Please open a database first.');
	}

	// Reload so the app picks up the restored database cleanly
	window.location.reload();
}
