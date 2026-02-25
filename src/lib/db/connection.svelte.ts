import initSqlJs, { type Database } from 'sql.js';
import { base } from '$app/paths';
import { CREATE_TABLES } from './schema.js';
import { migrateAddUuids } from './migrate.js';

// Raw (non-proxied) reference to the Database instance.
// Svelte 5's $state deep proxy wraps objects in a Proxy, which
// interferes with sql.js's internal WASM operations and parameter binding.
let _instance: Database | null = null;

export const db = $state<{
	instance: Database | null;
	fileName: string;
	fileHandle: FileSystemFileHandle | null;
	/** IndexedDB key when File System Access API is unavailable (Safari/Firefox) */
	idbKey: string | null;
	isOpen: boolean;
}>({
	instance: null,
	fileName: '',
	fileHandle: null,
	idbKey: null,
	isOpen: false
});

// --- IndexedDB helpers ---
const IDB_NAME = 'invoice-manager';
const IDB_STORE = 'file-handles';
const IDB_KEY = 'last-db-handle';
// Store for DB binary when File System Access API is unavailable
const IDB_DB_STORE = 'db-storage';
const IDB_META_KEY = '__meta__';

interface FSAccessHandle extends FileSystemFileHandle {
	queryPermission(opts: { mode: string }): Promise<string>;
	requestPermission(opts: { mode: string }): Promise<string>;
}

function openIDB(): Promise<IDBDatabase> {
	return new Promise((resolve, reject) => {
		const req = indexedDB.open(IDB_NAME, 2);
		req.onupgradeneeded = () => {
			const idb = req.result;
			if (!idb.objectStoreNames.contains(IDB_STORE)) {
				idb.createObjectStore(IDB_STORE);
			}
			if (!idb.objectStoreNames.contains(IDB_DB_STORE)) {
				idb.createObjectStore(IDB_DB_STORE);
			}
		};
		req.onsuccess = () => resolve(req.result);
		req.onerror = () => reject(req.error);
	});
}

function txComplete(tx: IDBTransaction): Promise<void> {
	return new Promise((resolve, reject) => {
		tx.oncomplete = () => resolve();
		tx.onerror = () => reject(tx.error);
	});
}

function reqResult<T>(req: IDBRequest): Promise<T> {
	return new Promise((resolve, reject) => {
		req.onsuccess = () => resolve(req.result as T);
		req.onerror = () => reject(req.error);
	});
}

async function storeHandle(handle: FileSystemFileHandle) {
	const idb = await openIDB();
	const tx = idb.transaction(IDB_STORE, 'readwrite');
	tx.objectStore(IDB_STORE).put(handle, IDB_KEY);
	await txComplete(tx);
	idb.close();
}

async function getStoredHandle(): Promise<FileSystemFileHandle | null> {
	try {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_STORE, 'readonly');
		const req = tx.objectStore(IDB_STORE).get(IDB_KEY);
		const result = await reqResult<FileSystemFileHandle | undefined>(req);
		idb.close();
		return result ?? null;
	} catch {
		return null;
	}
}

async function clearStoredHandle() {
	try {
		const idb = await openIDB();
		const tx1 = idb.transaction(IDB_STORE, 'readwrite');
		tx1.objectStore(IDB_STORE).delete(IDB_KEY);
		await txComplete(tx1);
		// Also clear IDB meta/data
		const tx2 = idb.transaction(IDB_DB_STORE, 'readwrite');
		tx2.objectStore(IDB_DB_STORE).delete(IDB_META_KEY);
		await txComplete(tx2);
		idb.close();
	} catch {
		// ignore
	}
}

// --- IDB binary storage (Safari/Firefox fallback) ---

async function storeDbBinary(key: string, data: Uint8Array, name: string) {
	const idb = await openIDB();
	const tx = idb.transaction(IDB_DB_STORE, 'readwrite');
	tx.objectStore(IDB_DB_STORE).put(data, key);
	tx.objectStore(IDB_DB_STORE).put({ key, name }, IDB_META_KEY);
	await txComplete(tx);
	idb.close();
}

async function loadDbBinary(key: string): Promise<Uint8Array | null> {
	try {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_DB_STORE, 'readonly');
		const req = tx.objectStore(IDB_DB_STORE).get(key);
		const result = await reqResult<Uint8Array | undefined>(req);
		idb.close();
		return result ?? null;
	} catch {
		return null;
	}
}

async function getStoredDbMeta(): Promise<{ key: string; name: string } | null> {
	try {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_DB_STORE, 'readonly');
		const req = tx.objectStore(IDB_DB_STORE).get(IDB_META_KEY);
		const meta = await reqResult<{ key: string; name: string } | undefined>(req);
		idb.close();
		return meta ?? null;
	} catch {
		return null;
	}
}

function supportsFileSystemAccess(): boolean {
	return typeof window !== 'undefined' && 'showSaveFilePicker' in window;
}

/**
 * Check if there's a stored handle we can reconnect to.
 * If permission is already granted, opens it silently.
 * Returns the stored file name if a handle exists but needs user activation, or null.
 */
export async function tryRestore(): Promise<string | null> {
	// Path 1: File System Access API (Chromium)
	if (supportsFileSystemAccess()) {
		const handle = await getStoredHandle();
		if (!handle) return null;

		const perm = await (handle as FSAccessHandle).queryPermission({ mode: 'readwrite' });
		if (perm === 'denied') {
			await clearStoredHandle();
			return null;
		}

		if (perm === 'granted') {
			const opened = await openFromHandle(handle);
			return opened ? null : null;
		}

		// Permission needs user gesture
		return handle.name;
	}

	// Path 2: IndexedDB binary storage (Safari/Firefox)
	const meta = await getStoredDbMeta();
	if (!meta) return null;

	const data = await loadDbBinary(meta.key);
	if (!data) return null;

	// Open silently — no permission needed for IndexedDB
	const opened = await openFromBinary(data, meta.name, meta.key);
	return opened ? null : null;
}

/** Reconnect to the stored handle (must be called from a user gesture). */
export async function reconnect(): Promise<boolean> {
	const handle = await getStoredHandle();
	if (!handle) return false;

	const granted = await (handle as FSAccessHandle).requestPermission({ mode: 'readwrite' });
	if (granted !== 'granted') return false;

	return openFromHandle(handle);
}

async function openFromHandle(handle: FileSystemFileHandle): Promise<boolean> {
	try {
		const SQL = await initSql();
		const file = await handle.getFile();
		const buffer = await file.arrayBuffer();
		const database = new SQL.Database(new Uint8Array(buffer));
		database.run('PRAGMA foreign_keys = ON;');
		database.run(CREATE_TABLES);
		_instance = database;
		migrateAddUuids();

		db.instance = database;
		db.fileName = handle.name;
		db.fileHandle = handle;
		db.idbKey = null;
		db.isOpen = true;
		return true;
	} catch {
		await clearStoredHandle();
		return false;
	}
}

async function openFromBinary(data: Uint8Array, name: string, idbKey: string): Promise<boolean> {
	try {
		const SQL = await initSql();
		const database = new SQL.Database(data);
		database.run('PRAGMA foreign_keys = ON;');
		database.run(CREATE_TABLES);
		_instance = database;
		migrateAddUuids();

		db.instance = database;
		db.fileName = name;
		db.fileHandle = null;
		db.idbKey = idbKey;
		db.isOpen = true;
		return true;
	} catch {
		await clearStoredHandle();
		return false;
	}
}

async function initSql() {
	return await initSqlJs({
		locateFile: () => `${base}/sql-wasm.wasm`
	});
}

export async function initNew() {
	const SQL = await initSql();

	let fileHandle: FileSystemFileHandle | null = null;
	let fileName = 'invoices.sqlite';
	let idbKey: string | null = null;

	if (supportsFileSystemAccess()) {
		fileHandle = await window.showSaveFilePicker({
			suggestedName: 'invoices.sqlite',
			types: [
				{
					description: 'SQLite Database',
					accept: { 'application/x-sqlite3': ['.sqlite', '.db'] }
				}
			]
		});
		fileName = fileHandle.name;
	} else {
		// Safari/Firefox: store in IndexedDB
		idbKey = 'idb:invoices.sqlite';
	}

	const database = new SQL.Database();
	database.run('PRAGMA foreign_keys = ON;');
	database.run(CREATE_TABLES);
	_instance = database;
	migrateAddUuids();

	db.instance = database;
	db.fileName = fileName;
	db.fileHandle = fileHandle;
	db.idbKey = idbKey;
	db.isOpen = true;

	if (fileHandle) await storeHandle(fileHandle);
	await save();
}

export async function openExisting() {
	const SQL = await initSql();

	if (supportsFileSystemAccess()) {
		const [fileHandle] = await window.showOpenFilePicker({
			types: [
				{
					description: 'SQLite Database',
					accept: { 'application/x-sqlite3': ['.sqlite', '.db'] }
				}
			]
		});
		const file = await fileHandle.getFile();
		const buffer = await file.arrayBuffer();
		const database = new SQL.Database(new Uint8Array(buffer));
		database.run('PRAGMA foreign_keys = ON;');
		database.run(CREATE_TABLES);
		_instance = database;
		migrateAddUuids();

		db.instance = database;
		db.fileName = fileHandle.name;
		db.fileHandle = fileHandle;
		db.idbKey = null;
		db.isOpen = true;

		await storeHandle(fileHandle);
	} else {
		// Safari/Firefox: pick via <input type="file">, then persist in IndexedDB
		const input = document.createElement('input');
		input.type = 'file';
		input.accept = '.sqlite,.db';

		const file = await new Promise<File>((resolve, reject) => {
			input.onchange = () => {
				if (input.files && input.files[0]) {
					resolve(input.files[0]);
				} else {
					reject(new Error('No file selected'));
				}
			};
			input.click();
		});

		const buffer = await file.arrayBuffer();
		const database = new SQL.Database(new Uint8Array(buffer));
		database.run('PRAGMA foreign_keys = ON;');
		database.run(CREATE_TABLES);
		_instance = database;
		migrateAddUuids();

		const idbKey = 'idb:' + file.name;
		db.instance = database;
		db.fileName = file.name;
		db.fileHandle = null;
		db.idbKey = idbKey;
		db.isOpen = true;

		// Persist immediately
		await save();
	}
}

export async function save() {
	if (!_instance) return;

	const data = _instance.export();

	if (db.fileHandle) {
		// Chromium: write directly to file
		const blob = new Blob([data as unknown as BlobPart], { type: 'application/x-sqlite3' });
		const writable = await db.fileHandle.createWritable();
		await writable.write(blob);
		await writable.close();
	} else if (db.idbKey) {
		// Safari/Firefox: persist in IndexedDB
		await storeDbBinary(db.idbKey, data, db.fileName);
	} else {
		// Last resort: trigger download
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
	db.fileHandle = null;
	db.idbKey = null;
	db.isOpen = false;
	await clearStoredHandle();
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
