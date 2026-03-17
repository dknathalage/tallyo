// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare const __PKG_VERSION__: string;

declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}

	interface FileSystemFileHandle {
		getFile(): Promise<File>;
		createWritable(): Promise<FileSystemWritableFileStream>;
	}

	interface FileSystemWritableFileStream extends WritableStream {
		write(data: BufferSource | Blob | string): Promise<void>;
		close(): Promise<void>;
	}

	interface Window {
		showOpenFilePicker(options?: {
			multiple?: boolean;
			types?: Array<{
				description?: string;
				accept: Record<string, string[]>;
			}>;
		}): Promise<FileSystemFileHandle[]>;
		showSaveFilePicker(options?: {
			suggestedName?: string;
			types?: Array<{
				description?: string;
				accept: Record<string, string[]>;
			}>;
		}): Promise<FileSystemFileHandle>;
	}
}

export {};
