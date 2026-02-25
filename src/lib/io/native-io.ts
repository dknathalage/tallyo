import { Filesystem, Directory, Encoding } from '@capacitor/filesystem';
import { Share } from '@capacitor/share';
import { Preferences } from '@capacitor/preferences';
import { FilePicker } from '@capawesome/capacitor-file-picker';
import type { FileIO } from './types.js';

const PREF_KEY = 'last-db-path';
const PREF_NAME_KEY = 'last-db-name';

export class NativeFileIO implements FileIO {
	async pickNewFile(suggestedName: string): Promise<string> {
		const result = await FilePicker.pickDirectory();
		const dirPath = result.path;
		const filePath = `${dirPath}/${suggestedName}`;

		// Create empty file at chosen location
		await Filesystem.writeFile({
			path: filePath,
			data: '',
			recursive: true
		});

		await this.storePath(filePath, suggestedName);
		return filePath;
	}

	async pickExistingFile(): Promise<string> {
		const result = await FilePicker.pickFiles({
			types: ['application/x-sqlite3', 'application/octet-stream'],
			limit: 1
		});

		const file = result.files[0];
		if (!file?.path) throw new Error('No file selected');

		await this.storePath(file.path, file.name);
		return file.path;
	}

	async readFile(handle: string): Promise<Uint8Array> {
		const result = await Filesystem.readFile({
			path: handle
		});

		// Capacitor returns base64 string for binary reads
		const base64 = result.data as string;
		const binary = atob(base64);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i++) {
			bytes[i] = binary.charCodeAt(i);
		}
		return bytes;
	}

	async writeFile(handle: string, data: Uint8Array): Promise<void> {
		// Convert Uint8Array to base64
		let binary = '';
		for (let i = 0; i < data.length; i++) {
			binary += String.fromCharCode(data[i]);
		}
		const base64 = btoa(binary);

		await Filesystem.writeFile({
			path: handle,
			data: base64,
			recursive: true
		});
	}

	async tryRestore(): Promise<{ handle: string; name: string } | null> {
		const { value: path } = await Preferences.get({ key: PREF_KEY });
		const { value: name } = await Preferences.get({ key: PREF_NAME_KEY });

		if (!path || !name) return null;

		try {
			// Verify file still exists
			await Filesystem.stat({ path });
			return { handle: path, name };
		} catch {
			await this.clearStored();
			return null;
		}
	}

	getFileName(handle: string): string {
		const parts = handle.split('/');
		return parts[parts.length - 1] ?? handle;
	}

	async exportBlob(blob: Blob, filename: string, mimeType: string): Promise<void> {
		// Convert blob to base64
		const arrayBuffer = await blob.arrayBuffer();
		const bytes = new Uint8Array(arrayBuffer);
		let binary = '';
		for (let i = 0; i < bytes.length; i++) {
			binary += String.fromCharCode(bytes[i]);
		}
		const base64 = btoa(binary);

		// Write to temp directory
		const tempPath = `tmp_exports/${filename}`;
		await Filesystem.writeFile({
			path: tempPath,
			data: base64,
			directory: Directory.Cache,
			recursive: true
		});

		// Get the full URI for sharing
		const stat = await Filesystem.getUri({
			path: tempPath,
			directory: Directory.Cache
		});

		await Share.share({
			title: filename,
			url: stat.uri,
			dialogTitle: `Share ${filename}`
		});
	}

	async clearStored(): Promise<void> {
		await Preferences.remove({ key: PREF_KEY });
		await Preferences.remove({ key: PREF_NAME_KEY });
	}

	// --- private helpers ---

	private async storePath(path: string, name: string): Promise<void> {
		await Preferences.set({ key: PREF_KEY, value: path });
		await Preferences.set({ key: PREF_NAME_KEY, value: name });
	}
}
