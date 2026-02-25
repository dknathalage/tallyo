export interface FileIO {
	/** Pick location and create a new empty DB file. Returns a handle/path identifier. */
	pickNewFile(suggestedName: string): Promise<string>;
	/** Pick an existing DB file. Returns a handle/path identifier. */
	pickExistingFile(): Promise<string>;
	/** Read the DB file as Uint8Array. */
	readFile(handle: string): Promise<Uint8Array>;
	/** Write the DB file from Uint8Array. */
	writeFile(handle: string, data: Uint8Array): Promise<void>;
	/** Try to restore a previously used file. Returns handle and name if available, null otherwise. */
	tryRestore(): Promise<{ handle: string; name: string } | null>;
	/** Get display name for a handle. */
	getFileName(handle: string): string;
	/** Share/download a blob (PDF, CSV). */
	exportBlob(blob: Blob, filename: string, mimeType: string): Promise<void>;
	/** Clear stored handle reference. */
	clearStored(): Promise<void>;
}
