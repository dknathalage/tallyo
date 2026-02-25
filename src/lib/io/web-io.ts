import type { FileIO } from './types.js';

// Extend FileSystemFileHandle with File System Access API methods
// that aren't in the base TypeScript DOM types yet
interface FSAccessHandle extends FileSystemFileHandle {
	queryPermission(opts: { mode: string }): Promise<string>;
	requestPermission(opts: { mode: string }): Promise<string>;
}

const IDB_NAME = 'invoice-manager';
const IDB_STORE = 'file-handles';
const IDB_KEY = 'last-db-handle';

function openIDB(): Promise<IDBDatabase> {
	return new Promise((resolve, reject) => {
		const req = indexedDB.open(IDB_NAME, 1);
		req.onupgradeneeded = () => {
			req.result.createObjectStore(IDB_STORE);
		};
		req.onsuccess = () => resolve(req.result);
		req.onerror = () => reject(req.error);
	});
}

function supportsFileSystemAccess(): boolean {
	return typeof window !== 'undefined' && 'showSaveFilePicker' in window;
}

/**
 * Web implementation of FileIO.
 *
 * Uses the File System Access API (showSaveFilePicker / showOpenFilePicker)
 * when available (Chromium browsers), with a fallback to <input type="file">
 * and <a> download links for other browsers.
 *
 * FileSystemFileHandle objects are stored in IndexedDB for restore.
 * The opaque string handle is the IDB_KEY constant — there's only ever one
 * stored handle on web, so we use it as a sentinel.
 */
export class WebFileIO implements FileIO {
	/** In-memory cache of the FileSystemFileHandle for the current session. */
	private fsHandle: FileSystemFileHandle | null = null;

	async pickNewFile(suggestedName: string): Promise<string> {
		if (supportsFileSystemAccess()) {
			this.fsHandle = await window.showSaveFilePicker({
				suggestedName,
				types: [
					{
						description: 'SQLite Database',
						accept: { 'application/x-sqlite3': ['.sqlite', '.db'] }
					}
				]
			});
			await this.storeHandle(this.fsHandle);
			return IDB_KEY;
		}
		// Non-FSAA browsers: no persistent handle — use a sentinel
		return '__memory__';
	}

	async pickExistingFile(): Promise<string> {
		if (supportsFileSystemAccess()) {
			const [handle] = await window.showOpenFilePicker({
				types: [
					{
						description: 'SQLite Database',
						accept: { 'application/x-sqlite3': ['.sqlite', '.db'] }
					}
				]
			});
			this.fsHandle = handle;
			await this.storeHandle(handle);
			return IDB_KEY;
		}

		// Fallback: <input type="file">
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

		// Stash the file data so readFile can return it
		this._fallbackFile = file;
		return '__fallback__';
	}

	/** Temporary storage for the file picked via the <input> fallback. */
	private _fallbackFile: File | null = null;

	async readFile(handle: string): Promise<Uint8Array> {
		if (handle === '__fallback__' && this._fallbackFile) {
			const buffer = await this._fallbackFile.arrayBuffer();
			return new Uint8Array(buffer);
		}

		const fsHandle = this.fsHandle ?? (await this.getStoredFsHandle());
		if (!fsHandle) throw new Error('No file handle available');

		this.fsHandle = fsHandle;
		const file = await fsHandle.getFile();
		const buffer = await file.arrayBuffer();
		return new Uint8Array(buffer);
	}

	async writeFile(handle: string, data: Uint8Array): Promise<void> {
		if (handle === '__memory__' || handle === '__fallback__') {
			// No persistent file handle — trigger a download
			const blob = new Blob([data as BlobPart], { type: 'application/x-sqlite3' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = 'invoices.sqlite';
			a.click();
			URL.revokeObjectURL(url);
			return;
		}

		const fsHandle = this.fsHandle ?? (await this.getStoredFsHandle());
		if (!fsHandle) throw new Error('No file handle available');

		this.fsHandle = fsHandle;
		const writable = await fsHandle.createWritable();
		await writable.write(new Blob([data as BlobPart], { type: 'application/x-sqlite3' }));
		await writable.close();
	}

	async tryRestore(): Promise<{ handle: string; name: string } | null> {
		if (!supportsFileSystemAccess()) return null;

		const handle = await this.getStoredFsHandle();
		if (!handle) return null;

		const perm = await (handle as FSAccessHandle).queryPermission({ mode: 'readwrite' });
		if (perm === 'denied') {
			await this.clearStored();
			return null;
		}

		if (perm === 'granted') {
			this.fsHandle = handle;
			return { handle: IDB_KEY, name: handle.name };
		}

		// Permission needs user gesture
		return { handle: IDB_KEY, name: handle.name };
	}

	/**
	 * Request readwrite permission on the stored handle.
	 * Must be called from a user gesture context.
	 */
	async requestPermission(): Promise<boolean> {
		const handle = this.fsHandle ?? (await this.getStoredFsHandle());
		if (!handle) return false;

		const granted = await (handle as FSAccessHandle).requestPermission({ mode: 'readwrite' });
		if (granted !== 'granted') return false;

		this.fsHandle = handle;
		return true;
	}

	getFileName(handle: string): string {
		if (handle === '__fallback__' && this._fallbackFile) {
			return this._fallbackFile.name;
		}
		return this.fsHandle?.name ?? 'invoices.sqlite';
	}

	async exportBlob(blob: Blob, filename: string, _mimeType: string): Promise<void> {
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = filename;
		a.click();
		URL.revokeObjectURL(url);
	}

	async clearStored(): Promise<void> {
		this.fsHandle = null;
		this._fallbackFile = null;
		try {
			const idb = await openIDB();
			const tx = idb.transaction(IDB_STORE, 'readwrite');
			tx.objectStore(IDB_STORE).delete(IDB_KEY);
			await new Promise<void>((resolve) => {
				tx.oncomplete = () => { idb.close(); resolve(); };
				tx.onerror = () => { idb.close(); resolve(); };
			});
		} catch {
			// ignore
		}
	}

	// --- private helpers ---

	private async storeHandle(handle: FileSystemFileHandle): Promise<void> {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_STORE, 'readwrite');
		tx.objectStore(IDB_STORE).put(handle, IDB_KEY);
		return new Promise<void>((resolve, reject) => {
			tx.oncomplete = () => { idb.close(); resolve(); };
			tx.onerror = () => { idb.close(); reject(tx.error); };
		});
	}

	private async getStoredFsHandle(): Promise<FileSystemFileHandle | null> {
		try {
			const idb = await openIDB();
			const tx = idb.transaction(IDB_STORE, 'readonly');
			const req = tx.objectStore(IDB_STORE).get(IDB_KEY);
			return new Promise((resolve) => {
				req.onsuccess = () => { idb.close(); resolve(req.result ?? null); };
				req.onerror = () => { idb.close(); resolve(null); };
			});
		} catch {
			return null;
		}
	}
}
