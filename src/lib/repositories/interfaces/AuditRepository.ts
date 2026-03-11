import type { AuditLogEntry } from '$lib/types/index.js';

export interface AuditRepository {
	getEntityHistory(entityType: string, entityId: number): AuditLogEntry[];
}
