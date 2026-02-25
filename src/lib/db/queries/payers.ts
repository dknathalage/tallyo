import { execute, query, save } from '../connection.svelte.js';
import { logAudit, computeChanges } from '../audit.js';
import type { Payer, PartySnapshot } from '../../types/index.js';

export function getPayers(search?: string): Payer[] {
	if (search) {
		return query<Payer>(
			'SELECT * FROM payers WHERE name LIKE ? OR email LIKE ? ORDER BY name',
			[`%${search}%`, `%${search}%`]
		);
	}
	return query<Payer>('SELECT * FROM payers ORDER BY name');
}

export function getPayer(id: number): Payer | null {
	const results = query<Payer>('SELECT * FROM payers WHERE id = ?', [id]);
	return results.length > 0 ? results[0] : null;
}

export async function createPayer(data: {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	metadata?: string;
}): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Payer name is required');
	}
	execute(
		'INSERT INTO payers (uuid, name, email, phone, address, metadata) VALUES (?, ?, ?, ?, ?, ?)',
		[crypto.randomUUID(), data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.metadata ?? '{}']
	);
	const result = query<{ id: number }>('SELECT last_insert_rowid() as id');
	logAudit({
		entity_type: 'payer',
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

export async function updatePayer(
	id: number,
	data: { name: string; email?: string; phone?: string; address?: string; metadata?: string }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Payer name is required');
	}
	const oldPayer = getPayer(id);
	execute(
		`UPDATE payers SET name = ?, email = ?, phone = ?, address = ?, metadata = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.metadata ?? '{}', id]
	);
	if (oldPayer) {
		const changes = computeChanges(
			oldPayer as unknown as Record<string, unknown>,
			{ name: data.name, email: data.email ?? '', phone: data.phone ?? '', address: data.address ?? '', metadata: data.metadata ?? '{}' },
			['name', 'email', 'phone', 'address', 'metadata']
		);
		if (Object.keys(changes).length > 0) {
			logAudit({ entity_type: 'payer', entity_id: id, action: 'update', changes });
		}
	}
	await save();
}

export async function deletePayer(id: number): Promise<void> {
	const payer = getPayer(id);
	execute('DELETE FROM payers WHERE id = ?', [id]);
	logAudit({ entity_type: 'payer', entity_id: id, action: 'delete', context: payer?.name ?? '' });
	await save();
}

export function buildPayerSnapshot(payerId: number | null): PartySnapshot {
	if (!payerId) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	const payer = getPayer(payerId);
	if (!payer) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	let metadata: Record<string, string> = {};
	try { metadata = JSON.parse(payer.metadata); } catch {}
	return {
		name: payer.name,
		email: payer.email,
		phone: payer.phone,
		address: payer.address,
		metadata
	};
}
