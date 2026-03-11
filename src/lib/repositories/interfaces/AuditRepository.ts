import type { AuditLogEntry } from '$lib/types/index.js';

export interface LogAuditParams {
	entity_type: string;
	entity_id?: number | null;
	action: string;
	changes?: Record<string, { old: unknown; new: unknown }>;
	context?: string;
	batch_id?: string;
}

export interface AuditRepository {
	/** Retrieve full history for a given entity. */
	getEntityHistory(entityType: string, entityId: number): AuditLogEntry[];
	/** Write a new audit log entry. */
	logAudit(params: LogAuditParams): void;
}
