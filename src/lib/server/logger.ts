/**
 * Structured logger. Writes to stderr with timestamps and scope tags.
 * Optionally mirrors to DATA_DIR/logs/tallyo.log if TALLYO_LOG_FILE != '0'.
 */
import { mkdirSync, appendFileSync } from 'node:fs';
import { join } from 'node:path';

type Level = 'debug' | 'info' | 'warn' | 'error';

const LEVEL_RANK: Record<Level, number> = { debug: 10, info: 20, warn: 30, error: 40 };

function resolveLevel(): Level {
	const raw = (process.env['LOG_LEVEL'] ?? 'info').toLowerCase();
	if (raw in LEVEL_RANK) return raw as Level;
	return 'info';
}

const minLevel = resolveLevel();
const minRank = LEVEL_RANK[minLevel];
const fileEnabled = process.env['TALLYO_LOG_FILE'] !== '0';

let logPath: string | null = null;
function getLogPath(): string | null {
	if (!fileEnabled) return null;
	if (logPath) return logPath;
	const base = process.env['DATA_DIR'];
	if (!base) return null;
	try {
		const dir = join(base, 'logs');
		mkdirSync(dir, { recursive: true });
		logPath = join(dir, 'tallyo.log');
		return logPath;
	} catch {
		return null;
	}
}

function safeStringify(value: unknown): string {
	if (value === undefined) return '';
	const seen = new WeakSet();
	try {
		return JSON.stringify(value, (_key, v) => {
			if (typeof v === 'object' && v !== null) {
				if (seen.has(v as object)) return '[Circular]';
				seen.add(v as object);
			}
			if (typeof v === 'bigint') return String(v);
			if (typeof v === 'function') return '[Function]';
			return v;
		});
	} catch {
		try {
			return String(value);
		} catch {
			return '[unserializable]';
		}
	}
}

function emit(level: Level, scope: string, message: string, data?: unknown): void {
	if (LEVEL_RANK[level] < minRank) return;
	const time = new Date().toISOString();
	const dataStr = data === undefined ? '' : ` ${safeStringify(data)}`;
	const line = `${time} [${level}] [${scope}] ${message}${dataStr}`;
	const stream = level === 'warn' || level === 'error' ? process.stderr : process.stdout;
	try {
		stream.write(line + '\n');
	} catch {
		// ignore
	}
	const file = getLogPath();
	if (file) {
		try {
			appendFileSync(file, line + '\n');
		} catch {
			// ignore
		}
	}
}

export function log(scope: string) {
	return {
		debug: (message: string, data?: unknown) => emit('debug', scope, message, data),
		info: (message: string, data?: unknown) => emit('info', scope, message, data),
		warn: (message: string, data?: unknown) => emit('warn', scope, message, data),
		error: (message: string, data?: unknown) => emit('error', scope, message, data),
		child: (sub: string) => log(`${scope}:${sub}`)
	};
}

export type Logger = ReturnType<typeof log>;

/**
 * Wraps an async fn so any thrown error is logged with scope before rethrow.
 */
export async function withErrorLog<T>(scope: string, op: string, fn: () => Promise<T>): Promise<T> {
	const l = log(scope);
	try {
		return await fn();
	} catch (e) {
		const message = e instanceof Error ? e.message : String(e);
		const stack = e instanceof Error ? e.stack : undefined;
		l.error(`${op} failed`, { error: message, stack });
		throw e;
	}
}
