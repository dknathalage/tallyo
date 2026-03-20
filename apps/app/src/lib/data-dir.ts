import { mkdirSync } from 'fs';
import { join } from 'path';
import { homedir } from 'os';

let _dataDir: string | null = null;

export function getDataDir(): string {
	if (_dataDir) return _dataDir;
	_dataDir = process.env.DATA_DIR || join(homedir(), '.tallyo');
	mkdirSync(_dataDir, { recursive: true });
	return _dataDir;
}

export function getDbPath(): string {
	return join(getDataDir(), 'tallyo.db');
}

export function getConfigPath(): string {
	return join(getDataDir(), 'config.json');
}
