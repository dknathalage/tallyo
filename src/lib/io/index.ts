import { isNative } from '$lib/platform';
import type { FileIO } from './types.js';

let _io: FileIO | null = null;

export async function getIO(): Promise<FileIO> {
	if (_io) return _io;
	if (isNative()) {
		const { NativeFileIO } = await import('./native-io.js');
		_io = new NativeFileIO();
	} else {
		const { WebFileIO } = await import('./web-io.js');
		_io = new WebFileIO();
	}
	return _io;
}

export type { FileIO } from './types.js';
