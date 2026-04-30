/**
 * Base interface for all storage repositories.
 * Provides a common contract that concrete implementations can extend.
 */
export interface StorageRepository {
	/** Persist any pending in-memory changes to durable storage. */
	save(): Promise<void>;
}
