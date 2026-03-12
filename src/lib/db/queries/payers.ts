import { execute, query } from '../connection.svelte.js';
import type { Payer, Client, PartySnapshot } from '../../types/index.js';

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

/**
 * Pure SQL: inserts a payer and returns the new id.
 * No audit logging, no save().
 */
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
	return result[0].id;
}

/**
 * Pure SQL: updates a payer.
 * No audit logging, no save().
 */
export async function updatePayer(
	id: number,
	data: { name: string; email?: string; phone?: string; address?: string; metadata?: string }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Payer name is required');
	}
	execute(
		`UPDATE payers SET name = ?, email = ?, phone = ?, address = ?, metadata = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.metadata ?? '{}', id]
	);
}

/**
 * Pure SQL: deletes a payer.
 * No audit logging, no save().
 */
export async function deletePayer(id: number): Promise<void> {
	execute('DELETE FROM payers WHERE id = ?', [id]);
}

/**
 * Pure SQL: bulk deletes payers.
 * No audit logging, no save().
 */
export async function bulkDeletePayers(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM payers WHERE id IN (${placeholders})`, ids);
}

export function getPayerClients(payerId: number): Client[] {
	return query<Client>('SELECT * FROM clients WHERE payer_id = ? ORDER BY name', [payerId]);
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
