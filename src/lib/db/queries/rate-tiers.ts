import { execute, query, save } from '../connection.svelte.js';
import type { RateTier } from '../../types/index.js';

export function getRateTiers(): RateTier[] {
	return query<RateTier>(`SELECT * FROM rate_tiers ORDER BY sort_order, name`);
}

export function getRateTier(id: number): RateTier | null {
	const results = query<RateTier>(`SELECT * FROM rate_tiers WHERE id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

export function getDefaultTier(): RateTier | null {
	const results = query<RateTier>(
		`SELECT * FROM rate_tiers ORDER BY sort_order, id LIMIT 1`
	);
	return results.length > 0 ? results[0] : null;
}

export async function createRateTier(data: {
	name: string;
	description?: string;
	sort_order?: number;
}): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Tier name is required');
	}
	execute(
		`INSERT INTO rate_tiers (uuid, name, description, sort_order) VALUES (?, ?, ?, ?)`,
		[crypto.randomUUID(), data.name, data.description ?? '', data.sort_order ?? 0]
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	await save();
	return result[0].id;
}

export async function updateRateTier(
	id: number,
	data: { name: string; description?: string; sort_order?: number }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Tier name is required');
	}
	execute(
		`UPDATE rate_tiers SET name = ?, description = ?, sort_order = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.description ?? '', data.sort_order ?? 0, id]
	);
	await save();
}

export async function deleteRateTier(id: number): Promise<void> {
	const tiers = getRateTiers();
	if (tiers.length <= 1) {
		throw new Error('Cannot delete the last tier');
	}
	execute(`DELETE FROM rate_tiers WHERE id = ?`, [id]);
	await save();
}
