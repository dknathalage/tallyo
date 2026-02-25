import { execute } from './connection.svelte.js';

export function logAudit(params: {
	entity_type: string;
	entity_id?: number | null;
	action: string;
	changes?: Record<string, { old: unknown; new: unknown }>;
	context?: string;
	batch_id?: string;
}): void {
	execute(
		`INSERT INTO audit_log (uuid, entity_type, entity_id, action, changes, context, batch_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			params.entity_type,
			params.entity_id ?? null,
			params.action,
			params.changes ? JSON.stringify(params.changes) : '{}',
			params.context ?? '',
			params.batch_id ?? null
		]
	);
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
