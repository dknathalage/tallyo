import { getDb } from '../connection.js';
import { taxRates } from '../drizzle-schema.js';
import { eq, ne, desc, asc } from 'drizzle-orm';
import type { TaxRate } from '../../types/index.js';

function mapRow(row: Record<string, unknown>): TaxRate {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		name: row.name as string,
		rate: row.rate as number,
		is_default: row.is_default === true ? 1 : 0,
		created_at:
			row.created_at instanceof Date
				? row.created_at.toISOString()
				: ((row.created_at as string) ?? ''),
		updated_at:
			row.updated_at instanceof Date
				? row.updated_at.toISOString()
				: ((row.updated_at as string) ?? '')
	};
}

export async function getTaxRates(): Promise<TaxRate[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(taxRates)
		.orderBy(desc(taxRates.is_default), asc(taxRates.name));
	return rows.map((r) => mapRow(r as Record<string, unknown>));
}

export async function getDefaultTaxRate(): Promise<TaxRate | null> {
	const db = getDb();
	const rows = await db
		.select()
		.from(taxRates)
		.where(eq(taxRates.is_default, true))
		.limit(1);
	return rows.length > 0 ? mapRow(rows[0] as Record<string, unknown>) : null;
}

export async function getTaxRate(id: number): Promise<TaxRate | null> {
	const db = getDb();
	const rows = await db.select().from(taxRates).where(eq(taxRates.id, id));
	return rows.length > 0 ? mapRow(rows[0] as Record<string, unknown>) : null;
}

export async function createTaxRate(data: {
	name: string;
	rate: number;
	is_default?: boolean;
}): Promise<number> {
	const db = getDb();

	if (data.is_default) {
		await db.update(taxRates).set({ is_default: false });
	}

	const result = await db
		.insert(taxRates)
		.values({
			uuid: crypto.randomUUID(),
			name: data.name,
			rate: data.rate,
			is_default: data.is_default ?? false
		})
		.returning({ id: taxRates.id });

	return result[0].id;
}

export async function updateTaxRate(
	id: number,
	data: { name: string; rate: number; is_default?: boolean }
): Promise<void> {
	const db = getDb();

	if (data.is_default) {
		await db
			.update(taxRates)
			.set({ is_default: false })
			.where(ne(taxRates.id, id));
	}

	await db
		.update(taxRates)
		.set({
			name: data.name,
			rate: data.rate,
			is_default: data.is_default ?? false,
			updated_at: new Date()
		})
		.where(eq(taxRates.id, id));
}

export async function deleteTaxRate(id: number): Promise<void> {
	const db = getDb();
	await db.delete(taxRates).where(eq(taxRates.id, id));
}
