import { error } from '@sveltejs/kit';

/**
 * Converts known SQLite/repository errors into proper HTTP errors.
 * Call inside catch blocks in API route handlers.
 * Re-throws unknown errors (will become 500).
 */
export function dbError(err: unknown): never {
	const msg = err instanceof Error ? err.message : String(err);

	if (msg.includes('UNIQUE constraint failed')) {
		const field = msg.split('UNIQUE constraint failed: ')[1]?.split('.')[1] ?? 'field';
		throw error(409, `A record with this ${field} already exists`);
	}
	if (msg.includes('FOREIGN KEY constraint failed')) {
		throw error(400, 'Invalid reference — check linked fields (tier, payer, client, etc.)');
	}
	if (msg.includes('NOT NULL constraint failed')) {
		const field = msg.split('NOT NULL constraint failed: ')[1]?.split('.')[1] ?? 'field';
		throw error(400, `${field} is required`);
	}
	if (msg.includes('is required') || msg.includes('Cannot delete') || msg.includes('Cannot convert')) {
		throw error(400, msg);
	}

	throw err;
}

/**
 * Normalizes FK fields from forms — treats 0, '', null, undefined as null.
 */
export function fkOrNull(val: unknown): number | null {
	const n = Number(val);
	return Number.isFinite(n) && n > 0 ? n : null;
}
