/**
 * Abstracts database transaction lifecycle.
 * Concrete implementations wrap the underlying storage engine's
 * BEGIN / COMMIT / ROLLBACK primitives.
 */
export interface StorageTransaction {
	/** Begin a new transaction. */
	begin(): Promise<void>;
	/** Commit the current transaction. */
	commit(): Promise<void>;
	/** Roll back the current transaction. */
	rollback(): Promise<void>;
	/**
	 * Execute `fn` inside an auto-managed transaction.
	 * Automatically calls begin(), then commit() on success
	 * or rollback() on error before re-throwing.
	 */
	run<T>(fn: () => Promise<T>): Promise<T>;
}
