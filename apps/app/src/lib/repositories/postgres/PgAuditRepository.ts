import { getEntityHistory } from '$lib/db/queries/audit.js';
import { logAudit } from '$lib/db/audit.js';
import type { AuditRepository, LogAuditParams } from '../interfaces/AuditRepository.js';
import type { AuditLogEntry } from '$lib/types/index.js';

export class PgAuditRepository implements AuditRepository {
	async getEntityHistory(entityType: string, entityId: number): Promise<AuditLogEntry[]> {
		return await getEntityHistory(entityType, entityId);
	}

	async logAudit(params: LogAuditParams): Promise<void> {
		await logAudit(params);
	}
}
