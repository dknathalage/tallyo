import { execute, query, save } from '../connection.svelte.js';
import { logAudit, computeChanges } from '../audit.js';
import type { Client } from '../../types/index.js';

export function getClients(search?: string): Client[] {
	if (search) {
		return query<Client>(
			`SELECT c.*, rt.name as pricing_tier_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id WHERE c.name LIKE ? OR c.email LIKE ? ORDER BY c.name`,
			[`%${search}%`, `%${search}%`]
		);
	}
	return query<Client>(`SELECT c.*, rt.name as pricing_tier_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id ORDER BY c.name`);
}

export function getClient(id: number): Client | null {
	const results = query<Client>(`SELECT c.*, rt.name as pricing_tier_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id WHERE c.id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

export async function createClient(data: {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	pricing_tier_id?: number | null;
}): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	execute(
		`INSERT INTO clients (uuid, name, email, phone, address, pricing_tier_id) VALUES (?, ?, ?, ?, ?, ?)`,
		[crypto.randomUUID(), data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.pricing_tier_id ?? null]
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	logAudit({
		entity_type: 'client',
		entity_id: result[0].id,
		action: 'create',
		changes: {
			name: { old: null, new: data.name },
			email: { old: null, new: data.email ?? '' }
		}
	});
	await save();
	return result[0].id;
}

export async function updateClient(
	id: number,
	data: { name: string; email?: string; phone?: string; address?: string; pricing_tier_id?: number | null }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	const oldClient = getClient(id);
	execute(
		`UPDATE clients SET name = ?, email = ?, phone = ?, address = ?, pricing_tier_id = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.pricing_tier_id ?? null, id]
	);
	if (oldClient) {
		const changes = computeChanges(
			oldClient as unknown as Record<string, unknown>,
			{ name: data.name, email: data.email ?? '', phone: data.phone ?? '', address: data.address ?? '', pricing_tier_id: data.pricing_tier_id ?? null },
			['name', 'email', 'phone', 'address', 'pricing_tier_id']
		);
		if (Object.keys(changes).length > 0) {
			logAudit({ entity_type: 'client', entity_id: id, action: 'update', changes });
		}
	}
	await save();
}

export async function deleteClient(id: number): Promise<void> {
	const client = getClient(id);
	execute(`DELETE FROM clients WHERE id = ?`, [id]);
	logAudit({ entity_type: 'client', entity_id: id, action: 'delete', context: client?.name ?? '' });
	await save();
}

export async function bulkDeleteClients(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const batch_id = crypto.randomUUID();
	const clients = ids.map((id) => getClient(id));
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM clients WHERE id IN (${placeholders})`, ids);
	for (let i = 0; i < ids.length; i++) {
		logAudit({ entity_type: 'client', entity_id: ids[i], action: 'delete', context: clients[i]?.name ?? '', batch_id });
	}
	await save();
}
