import { getEntityHistory } from '$lib/db/queries/audit.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { AuditLogEntry } from '$lib/types/index.js';

export class SqliteAuditRepository implements AuditRepository {
	getEntityHistory(entityType: string, entityId: number): AuditLogEntry[] {
		return getEntityHistory(entityType, entityId);
	}
}
