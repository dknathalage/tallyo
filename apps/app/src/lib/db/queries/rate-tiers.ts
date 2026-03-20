import { getDb } from '../connection.js';
import { rateTiers } from '../drizzle-schema.js';
import { eq, asc, sql } from 'drizzle-orm';
import type { RateTier } from '../../types/index.js';

function mapRow(row: Record<string, unknown>): RateTier {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		name: row.name as string,
		description: (row.description as string) ?? '',
		sort_order: (row.sort_order as number) ?? 0,
		created_at: (row.created_at as string) ?? '',
		updated_at: (row.updated_at as string) ?? ''
	};
}

export async function getRateTiers(): Promise<RateTier[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(rateTiers)
		.orderBy(asc(rateTiers.sort_order), asc(rateTiers.name));
	return rows.map((r) => mapRow(r as Record<string, unknown>));
}

export async function getRateTier(id: number): Promise<RateTier | null> {
	const db = getDb();
	const rows = await db.select().from(rateTiers).where(eq(rateTiers.id, id));
	return rows.length > 0 ? mapRow(rows[0] as Record<string, unknown>) : null;
}

export async function getDefaultTier(): Promise<RateTier | null> {
	const db = getDb();
	const rows = await db
		.select()
		.from(rateTiers)
		.orderBy(asc(rateTiers.sort_order), asc(rateTiers.id))
		.limit(1);
	return rows.length > 0 ? mapRow(rows[0] as Record<string, unknown>) : null;
}

export async function createRateTier(data: {
	name: string;
	description?: string;
	sort_order?: number;
}): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Tier name is required');
	}
	const db = getDb();

	const result = await db
		.insert(rateTiers)
		.values({
			uuid: crypto.randomUUID(),
			name: data.name,
			description: data.description ?? '',
			sort_order: data.sort_order ?? 0
		})
		.returning({ id: rateTiers.id });

	return result[0].id;
}

export async function updateRateTier(
	id: number,
	data: { name: string; description?: string; sort_order?: number }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Tier name is required');
	}
	const db = getDb();

	await db
		.update(rateTiers)
		.set({
			name: data.name,
			description: data.description ?? '',
			sort_order: data.sort_order ?? 0,
			updated_at: new Date().toISOString()
		})
		.where(eq(rateTiers.id, id));
}

export async function deleteRateTier(id: number): Promise<void> {
	const db = getDb();

	const countResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(rateTiers);

	if (countResult[0].count <= 1) {
		throw new Error('Cannot delete the last tier');
	}

	await db.delete(rateTiers).where(eq(rateTiers.id, id));
}
