import type { AuditLogEntry } from '$lib/types/index.js';
import type { AuditEntityType, AuditAction } from '$lib/db/audit.js';

export interface LogAuditParams {
	entity_type: AuditEntityType;
	entity_id?: number | null | undefined;
	action: AuditAction;
	changes?: Record<string, { old: unknown; new: unknown }> | undefined;
	context?: string | undefined;
	batch_id?: string | undefined;
}

export interface AuditRepository {
	/** Retrieve full history for a given entity. */
	getEntityHistory(entityType: string, entityId: number): Promise<AuditLogEntry[]>;
	/** Write a new audit log entry. */
	logAudit(params: LogAuditParams): Promise<void>;
}
