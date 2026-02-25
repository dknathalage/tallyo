import type { FileIO } from './types.js';

// Extend FileSystemFileHandle with File System Access API methods
// that aren't in the base TypeScript DOM types yet
interface FSAccessHandle extends FileSystemFileHandle {
	queryPermission(opts: { mode: string }): Promise<string>;
	requestPermission(opts: { mode: string }): Promise<string>;
}

// --- IndexedDB constants ---
const IDB_NAME = 'invoice-manager';
const IDB_STORE = 'file-handles';
const IDB_KEY = 'last-db-handle';
// Store for the fallback DB binary (Safari/Firefox)
const IDB_DB_STORE = 'db-storage';
const IDB_DB_KEY_PREFIX = 'idb:';

function openIDB(): Promise<IDBDatabase> {
	return new Promise((resolve, reject) => {
		const req = indexedDB.open(IDB_NAME, 2);
		req.onupgradeneeded = () => {
			const db = req.result;
			if (!db.objectStoreNames.contains(IDB_STORE)) {
				db.createObjectStore(IDB_STORE);
			}
			if (!db.objectStoreNames.contains(IDB_DB_STORE)) {
				db.createObjectStore(IDB_DB_STORE);
			}
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
 * **Chromium browsers** — Uses the File System Access API
 * (showSaveFilePicker / showOpenFilePicker) for direct file read/write.
 * FileSystemFileHandle objects are stored in IndexedDB for session restore.
 *
 * **Safari / Firefox (no File System Access API)** — Uses IndexedDB to
 * transparently persist the SQLite database binary. The database survives
 * page reloads without requiring file downloads on every save. Users can
 * still import an existing .sqlite file via <input type="file">.
 */
export class WebFileIO implements FileIO {
	/** In-memory cache of the FileSystemFileHandle for the current session. */
	private fsHandle: FileSystemFileHandle | null = null;

	// ─── File System Access API path (Chromium) ───────────────────────

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
			await this.storeFsHandle(this.fsHandle);
			return IDB_KEY;
		}

		// Safari/Firefox: create a new DB stored in IndexedDB
		const handle = IDB_DB_KEY_PREFIX + suggestedName;
		await this.storeIdbMeta(handle, suggestedName);
		return handle;
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
			await this.storeFsHandle(handle);
			return IDB_KEY;
		}

		// Safari/Firefox: pick via <input type="file">, then store in IndexedDB
		const file = await this.pickFileViaInput();
		const handle = IDB_DB_KEY_PREFIX + file.name;
		const buffer = await file.arrayBuffer();
		await this.writeIdbData(handle, new Uint8Array(buffer));
		await this.storeIdbMeta(handle, file.name);
		return handle;
	}

	async readFile(handle: string): Promise<Uint8Array> {
		if (this.isIdbHandle(handle)) {
			return await this.readIdbData(handle);
		}

		const fsHandle = this.fsHandle ?? (await this.getStoredFsHandle());
		if (!fsHandle) throw new Error('No file handle available');

		this.fsHandle = fsHandle;
		const file = await fsHandle.getFile();
		const buffer = await file.arrayBuffer();
		return new Uint8Array(buffer);
	}

	async writeFile(handle: string, data: Uint8Array): Promise<void> {
		if (this.isIdbHandle(handle)) {
			await this.writeIdbData(handle, data);
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
		// Try File System Access API restore first (Chromium)
		if (supportsFileSystemAccess()) {
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

		// Safari/Firefox: check for stored DB in IndexedDB
		return await this.getIdbMeta();
	}

	/**
	 * Request readwrite permission on the stored handle.
	 * Must be called from a user gesture context.
	 * Only relevant for File System Access API (Chromium).
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
		if (this.isIdbHandle(handle)) {
			return handle.slice(IDB_DB_KEY_PREFIX.length);
		}
		return this.fsHandle?.name ?? 'invoices.sqlite';
	}

	async exportBlob(blob: Blob, filename: string, _mimeType: string): Promise<void> {
		// Use Web Share API on mobile browsers that support it (iOS Safari)
		if (this.isMobileBrowser() && navigator.canShare?.({ files: [new File([blob], filename)] })) {
			try {
				await navigator.share({
					files: [new File([blob], filename, { type: _mimeType })],
					title: filename
				});
				return;
			} catch (e) {
				// User cancelled or share failed — fall through to download
				if (e instanceof Error && e.name === 'AbortError') return;
			}
		}

		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = filename;
		a.click();
		URL.revokeObjectURL(url);
	}

	async clearStored(): Promise<void> {
		this.fsHandle = null;
		try {
			const idb = await openIDB();

			// Clear FSAA handle
			const tx1 = idb.transaction(IDB_STORE, 'readwrite');
			tx1.objectStore(IDB_STORE).delete(IDB_KEY);
			await txComplete(tx1);

			// Clear IDB meta
			const tx2 = idb.transaction(IDB_DB_STORE, 'readwrite');
			tx2.objectStore(IDB_DB_STORE).delete('__meta__');
			await txComplete(tx2);

			idb.close();
		} catch {
			// ignore
		}
	}

	// ─── Private: File System Access API helpers ──────────────────────

	private async storeFsHandle(handle: FileSystemFileHandle): Promise<void> {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_STORE, 'readwrite');
		tx.objectStore(IDB_STORE).put(handle, IDB_KEY);
		await txComplete(tx);
		idb.close();
	}

	private async getStoredFsHandle(): Promise<FileSystemFileHandle | null> {
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

	// ─── Private: IndexedDB fallback helpers (Safari/Firefox) ─────────

	private isIdbHandle(handle: string): boolean {
		return handle.startsWith(IDB_DB_KEY_PREFIX);
	}

	private async writeIdbData(handle: string, data: Uint8Array): Promise<void> {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_DB_STORE, 'readwrite');
		tx.objectStore(IDB_DB_STORE).put(data, handle);
		await txComplete(tx);
		idb.close();
	}

	private async readIdbData(handle: string): Promise<Uint8Array> {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_DB_STORE, 'readonly');
		const req = tx.objectStore(IDB_DB_STORE).get(handle);
		const result = await reqResult<Uint8Array | undefined>(req);
		idb.close();
		if (!result) throw new Error('No database found in storage');
		return result;
	}

	/** Store which IDB handle + display name was last used, for tryRestore. */
	private async storeIdbMeta(handle: string, name: string): Promise<void> {
		const idb = await openIDB();
		const tx = idb.transaction(IDB_DB_STORE, 'readwrite');
		tx.objectStore(IDB_DB_STORE).put({ handle, name }, '__meta__');
		await txComplete(tx);
		idb.close();
	}

	/** Retrieve last-used IDB handle + name. */
	private async getIdbMeta(): Promise<{ handle: string; name: string } | null> {
		try {
			const idb = await openIDB();
			const tx = idb.transaction(IDB_DB_STORE, 'readonly');
			const req = tx.objectStore(IDB_DB_STORE).get('__meta__');
			const meta = await reqResult<{ handle: string; name: string } | undefined>(req);
			idb.close();
			if (!meta) return null;

			// Verify the data actually exists
			const idb2 = await openIDB();
			const tx2 = idb2.transaction(IDB_DB_STORE, 'readonly');
			const dataReq = tx2.objectStore(IDB_DB_STORE).get(meta.handle);
			const data = await reqResult<Uint8Array | undefined>(dataReq);
			idb2.close();

			return data ? meta : null;
		} catch {
			return null;
		}
	}

	// ─── Private: input fallback ──────────────────────────────────────

	private pickFileViaInput(): Promise<File> {
		const input = document.createElement('input');
		input.type = 'file';
		input.accept = '.sqlite,.db';

		return new Promise<File>((resolve, reject) => {
			input.onchange = () => {
				if (input.files && input.files[0]) {
					resolve(input.files[0]);
				} else {
					reject(new Error('No file selected'));
				}
			};
			input.click();
		});
	}

	private isMobileBrowser(): boolean {
		return /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);
	}
}

// ─── IndexedDB promise helpers ────────────────────────────────────────

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
