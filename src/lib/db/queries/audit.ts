import { query } from '../connection.js';
import type { AuditLogEntry } from '../../types/index.js';

export function getEntityHistory(entity_type: string, entity_id: number): AuditLogEntry[] {
	return query<AuditLogEntry>(
		'SELECT * FROM audit_log WHERE entity_type = ? AND entity_id = ? ORDER BY created_at DESC',
		[entity_type, entity_id]
	);
}
