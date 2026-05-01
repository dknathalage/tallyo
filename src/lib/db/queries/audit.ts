import { getDb } from '../connection.js';
import { auditLog } from '../drizzle-schema.js';
import { desc, eq, and } from 'drizzle-orm';
import type { AuditLogEntry } from '../../types/index.js';

export async function getEntityHistory(entity_type: string, entity_id: number): Promise<AuditLogEntry[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(auditLog)
		.where(and(eq(auditLog.entity_type, entity_type), eq(auditLog.entity_id, entity_id)))
		.orderBy(desc(auditLog.created_at));
	return rows.map(mapAuditRow);
}

function mapAuditRow(row: typeof auditLog.$inferSelect): AuditLogEntry {
	return {
		id: row.id,
		uuid: row.uuid,
		entity_type: row.entity_type,
		entity_id: row.entity_id,
		action: row.action,
		changes: row.changes ?? '{}',
		context: row.context ?? '',
		batch_id: row.batch_id ?? null,
		created_at: row.created_at ?? ''
	};
}
