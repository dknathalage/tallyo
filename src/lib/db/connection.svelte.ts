import initSqlJs, { type Database } from 'sql.js';
import { base } from '$app/paths';
import { CREATE_TABLES } from './schema.js';

// Raw (non-proxied) reference to the Database instance.
// Svelte 5's $state deep proxy wraps objects in a Proxy, which
// interferes with sql.js's internal WASM operations and parameter binding.
let _instance: Database | null = null;

export const db = $state<{
	instance: Database | null;
	fileName: string;
	fileHandle: FileSystemFileHandle | null;
	isOpen: boolean;
}>({
	instance: null,
	fileName: '',
	fileHandle: null,
	isOpen: false
});

async function initSql() {
	return await initSqlJs({
		locateFile: () => `${base}/sql-wasm.wasm`
	});
}

function supportsFileSystemAccess(): boolean {
	return typeof window !== 'undefined' && 'showSaveFilePicker' in window;
}

export async function initNew() {
	const SQL = await initSql();

	let fileHandle: FileSystemFileHandle | null = null;
	let fileName = 'invoices.sqlite';

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
	}

	const database = new SQL.Database();
	database.run('PRAGMA foreign_keys = ON;');
	database.run(CREATE_TABLES);

	_instance = database;
	db.instance = database;
	db.fileName = fileName;
	db.fileHandle = fileHandle;
	db.isOpen = true;

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

		_instance = database;
		db.instance = database;
		db.fileName = fileHandle.name;
		db.fileHandle = fileHandle;
		db.isOpen = true;
	} else {
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

		_instance = database;
		db.instance = database;
		db.fileName = file.name;
		db.fileHandle = null;
		db.isOpen = true;
	}
}

export async function save() {
	if (!_instance) return;

	const data = _instance.export();
	const blob = new Blob([data as unknown as BlobPart], { type: 'application/x-sqlite3' });

	if (db.fileHandle) {
		const writable = await db.fileHandle.createWritable();
		await writable.write(blob);
		await writable.close();
	} else {
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = db.fileName || 'invoices.sqlite';
		a.click();
		URL.revokeObjectURL(url);
	}
}

export function close() {
	if (_instance) {
		_instance.close();
	}
	_instance = null;
	db.instance = null;
	db.fileName = '';
	db.fileHandle = null;
	db.isOpen = false;
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
