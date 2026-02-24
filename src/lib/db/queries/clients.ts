import { execute, query, save } from '../connection.svelte.js';
import type { Client } from '../../types/index.js';

export function getClients(search?: string): Client[] {
	if (search) {
		return query<Client>(
			`SELECT * FROM clients WHERE name LIKE ? OR email LIKE ? ORDER BY name`,
			[`%${search}%`, `%${search}%`]
		);
	}
	return query<Client>(`SELECT * FROM clients ORDER BY name`);
}

export function getClient(id: number): Client | null {
	const results = query<Client>(`SELECT * FROM clients WHERE id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

export async function createClient(data: {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
}): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	execute(
		`INSERT INTO clients (name, email, phone, address) VALUES (?, ?, ?, ?)`,
		[data.name, data.email ?? '', data.phone ?? '', data.address ?? '']
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	await save();
	return result[0].id;
}

export async function updateClient(
	id: number,
	data: { name: string; email?: string; phone?: string; address?: string }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	execute(
		`UPDATE clients SET name = ?, email = ?, phone = ?, address = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.email ?? '', data.phone ?? '', data.address ?? '', id]
	);
	await save();
}

export async function deleteClient(id: number): Promise<void> {
	execute(`DELETE FROM clients WHERE id = ?`, [id]);
	await save();
}
