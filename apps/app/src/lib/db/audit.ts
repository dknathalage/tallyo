import { getDb } from './connection.js';
import { auditLog } from './drizzle-schema.js';

export type AuditEntityType =
	| 'invoice'
	| 'estimate'
	| 'client'
	| 'payer'
	| 'catalog'
	| 'catalog_item'
	| 'rate_tier'
	| 'tax_rate'
	| 'payment'
	| 'recurring_template'
	| 'business_profile';

export type AuditAction =
	| 'create'
	| 'update'
	| 'delete'
	| 'status_change'
	| 'convert'
	| 'duplicate'
	| 'bulk_delete'
	| 'import'
	| 'export'
	| 'backup'
	| 'restore';

export async function logAudit(params: {
	entity_type: AuditEntityType;
	entity_id?: number | null;
	action: AuditAction;
	changes?: Record<string, { old: unknown; new: unknown }>;
	context?: string;
	batch_id?: string;
}): Promise<void> {
	const db = getDb();
	await db.insert(auditLog).values({
		entity_type: params.entity_type,
		entity_id: params.entity_id ?? null,
		action: params.action,
		changes: params.changes ? JSON.stringify(params.changes) : '{}',
		context: params.context ?? '',
		batch_id: params.batch_id ?? null
	});
}

export function computeChanges(
	oldObj: Record<string, unknown>,
	newObj: Record<string, unknown>,
	fields: string[]
): Record<string, { old: unknown; new: unknown }> {
	const changes: Record<string, { old: unknown; new: unknown }> = {};
	for (const field of fields) {
		const oldVal = oldObj[field];
		const newVal = newObj[field];
		if (oldVal !== newVal) {
			changes[field] = { old: oldVal, new: newVal };
		}
	}
	return changes;
}
