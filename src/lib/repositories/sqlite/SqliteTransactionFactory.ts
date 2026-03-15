import { runRaw } from '$lib/db/connection.js';
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';

/**
 * SQLite-backed transaction implementation.
 * Wraps the database connection's runRaw() with BEGIN / COMMIT / ROLLBACK.
 */
export class SqliteTransaction implements StorageTransaction {
	async begin(): Promise<void> {
		runRaw('BEGIN TRANSACTION');
	}

	async commit(): Promise<void> {
		runRaw('COMMIT');
	}

	async rollback(): Promise<void> {
		runRaw('ROLLBACK');
	}

	async run<T>(fn: () => Promise<T>): Promise<T> {
		await this.begin();
		try {
			const result = await fn();
			await this.commit();
			return result;
		} catch (e) {
			await this.rollback();
			throw e;
		}
	}
}

/**
 * Factory that creates SqliteTransaction instances.
 * Inject this into repositories that need transactional semantics.
 */
export class SqliteTransactionFactory {
	create(): StorageTransaction {
		return new SqliteTransaction();
	}
}
