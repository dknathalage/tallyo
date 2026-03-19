import { getDb } from '../connection.js';
import { catalogItems, catalogItemRates } from '../drizzle-schema.js';
import { eq, like, or, and, ne, inArray, asc, sql } from 'drizzle-orm';
import type {
	CatalogItem,
	CatalogItemWithRates,
	PaginationParams,
	PaginatedResult
} from '../../types/index.js';
import { paginate } from '../../types/index.js';

function mapRow(row: Record<string, unknown>): CatalogItem {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		name: row.name as string,
		rate: (row.rate as number) ?? 0,
		unit: (row.unit as string) ?? '',
		category: (row.category as string) ?? '',
		sku: (row.sku as string) ?? '',
		metadata: (row.metadata as string) ?? '{}',
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

export async function getCatalogItems(
	search?: string,
	category?: string,
	pagination?: PaginationParams
): Promise<PaginatedResult<CatalogItem>> {
	const db = getDb();

	const conditions = [];
	if (search) {
		conditions.push(
			or(like(catalogItems.name, `%${search}%`), like(catalogItems.sku, `%${search}%`))!
		);
	}
	if (category) {
		conditions.push(eq(catalogItems.category, category));
	}

	let query;
	if (conditions.length > 0) {
		query = db
			.select()
			.from(catalogItems)
			.where(and(...conditions))
			.orderBy(asc(catalogItems.name));
	} else {
		query = db.select().from(catalogItems).orderBy(asc(catalogItems.name));
	}

	const rows = await query;
	const all = rows.map(mapRow);
	return paginate(all, pagination);
}

export async function getCatalogItem(id: number): Promise<CatalogItem | null> {
	const db = getDb();
	const rows = await db.select().from(catalogItems).where(eq(catalogItems.id, id));
	return rows.length > 0 ? mapRow(rows[0]) : null;
}

export async function getCatalogCategories(): Promise<string[]> {
	const db = getDb();
	const rows = await db
		.selectDistinct({ category: catalogItems.category })
		.from(catalogItems)
		.where(ne(catalogItems.category, ''))
		.orderBy(asc(catalogItems.category));
	return rows.map((r) => r.category ?? '');
}

export async function searchCatalogItems(
	term: string,
	limit: number = 10
): Promise<CatalogItem[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(catalogItems)
		.where(or(like(catalogItems.name, `%${term}%`), like(catalogItems.sku, `%${term}%`)))
		.orderBy(asc(catalogItems.name))
		.limit(limit);
	return rows.map(mapRow);
}

/**
 * Inserts a catalog item and returns the new id.
 */
export async function createCatalogItem(data: {
	name: string;
	rate?: number;
	unit?: string;
	category?: string;
	sku?: string;
}): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Catalog item name is required');
	}
	const db = getDb();

	const result = await db
		.insert(catalogItems)
		.values({
			uuid: crypto.randomUUID(),
			name: data.name,
			rate: data.rate ?? 0,
			unit: data.unit ?? '',
			category: data.category ?? '',
			sku: data.sku ?? ''
		})
		.returning({ id: catalogItems.id });

	return result[0].id;
}

/**
 * Updates a catalog item.
 */
export async function updateCatalogItem(
	id: number,
	data: { name: string; rate?: number; unit?: string; category?: string; sku?: string }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Catalog item name is required');
	}
	const db = getDb();

	await db
		.update(catalogItems)
		.set({
			name: data.name,
			rate: data.rate ?? 0,
			unit: data.unit ?? '',
			category: data.category ?? '',
			sku: data.sku ?? '',
			updated_at: new Date()
		})
		.where(eq(catalogItems.id, id));
}

/**
 * Deletes a catalog item.
 */
export async function deleteCatalogItem(id: number): Promise<void> {
	const db = getDb();
	await db.delete(catalogItems).where(eq(catalogItems.id, id));
}

/**
 * Bulk deletes catalog items.
 */
export async function bulkDeleteCatalogItems(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db.delete(catalogItems).where(inArray(catalogItems.id, ids));
}

export async function getCatalogItemWithRates(
	id: number
): Promise<CatalogItemWithRates | null> {
	const db = getDb();

	const rows = await db
		.select({
			id: catalogItems.id,
			uuid: catalogItems.uuid,
			name: catalogItems.name,
			rate: catalogItems.rate,
			unit: catalogItems.unit,
			category: catalogItems.category,
			sku: catalogItems.sku,
			metadata: catalogItems.metadata,
			created_at: catalogItems.created_at,
			updated_at: catalogItems.updated_at,
			tier_id: catalogItemRates.rate_tier_id,
			tier_rate: catalogItemRates.rate
		})
		.from(catalogItems)
		.leftJoin(catalogItemRates, eq(catalogItems.id, catalogItemRates.catalog_item_id))
		.where(eq(catalogItems.id, id));

	if (rows.length === 0) return null;

	const item = rows[0];
	const rates: Record<number, number> = {};
	for (const row of rows) {
		if (row.tier_id != null && row.tier_rate != null) {
			rates[row.tier_id] = row.tier_rate;
		}
	}

	return {
		id: item.id,
		uuid: item.uuid as string,
		name: item.name,
		rate: item.rate,
		unit: item.unit ?? '',
		category: item.category ?? '',
		sku: item.sku ?? '',
		metadata: item.metadata ?? '{}',
		created_at:
			item.created_at instanceof Date ? item.created_at.toISOString() : ((item.created_at as string | null) ?? ''),
		updated_at:
			item.updated_at instanceof Date ? item.updated_at.toISOString() : ((item.updated_at as string | null) ?? ''),
		rates
	};
}

export async function getCatalogItemsWithTierRate(
	search?: string,
	category?: string,
	tierId?: number
): Promise<(CatalogItem & { tier_rate?: number })[]> {
	const db = getDb();

	const conditions = [];
	if (search) {
		conditions.push(
			or(like(catalogItems.name, `%${search}%`), like(catalogItems.sku, `%${search}%`))!
		);
	}
	if (category) {
		conditions.push(eq(catalogItems.category, category));
	}

	let query;
	if (tierId) {
		const baseQuery = db
			.select({
				id: catalogItems.id,
				uuid: catalogItems.uuid,
				name: catalogItems.name,
				rate: catalogItems.rate,
				unit: catalogItems.unit,
				category: catalogItems.category,
				sku: catalogItems.sku,
				metadata: catalogItems.metadata,
				created_at: catalogItems.created_at,
				updated_at: catalogItems.updated_at,
				tier_rate: catalogItemRates.rate
			})
			.from(catalogItems)
			.leftJoin(
				catalogItemRates,
				and(
					eq(catalogItems.id, catalogItemRates.catalog_item_id),
					eq(catalogItemRates.rate_tier_id, tierId)
				)
			);

		if (conditions.length > 0) {
			query = baseQuery.where(and(...conditions));
		} else {
			query = baseQuery;
		}
	} else {
		const baseQuery = db
			.select({
				id: catalogItems.id,
				uuid: catalogItems.uuid,
				name: catalogItems.name,
				rate: catalogItems.rate,
				unit: catalogItems.unit,
				category: catalogItems.category,
				sku: catalogItems.sku,
				metadata: catalogItems.metadata,
				created_at: catalogItems.created_at,
				updated_at: catalogItems.updated_at
			})
			.from(catalogItems);

		if (conditions.length > 0) {
			query = baseQuery.where(and(...conditions));
		} else {
			query = baseQuery;
		}
	}

	const rows = await query.orderBy(asc(catalogItems.name));

	return rows.map((row) => ({
		...mapRow(row as Record<string, unknown>),
		tier_rate: 'tier_rate' in row ? (row.tier_rate as number | undefined) ?? undefined : undefined
	}));
}

export async function getEffectiveRate(
	catalogItemId: number,
	tierId: number | null
): Promise<number> {
	const db = getDb();

	if (tierId) {
		const results = await db
			.select({ rate: catalogItemRates.rate })
			.from(catalogItemRates)
			.where(
				and(
					eq(catalogItemRates.catalog_item_id, catalogItemId),
					eq(catalogItemRates.rate_tier_id, tierId)
				)
			);
		if (results.length > 0) {
			return results[0].rate;
		}
	}

	const item = await getCatalogItem(catalogItemId);
	return item?.rate ?? 0;
}

/**
 * Sets a catalog item rate for a tier (upsert).
 */
export async function setCatalogItemRate(
	catalogItemId: number,
	tierId: number,
	rate: number
): Promise<void> {
	const db = getDb();

	await db
		.insert(catalogItemRates)
		.values({
			catalog_item_id: catalogItemId,
			rate_tier_id: tierId,
			rate
		})
		.onConflictDoUpdate({
			target: [catalogItemRates.catalog_item_id, catalogItemRates.rate_tier_id],
			set: { rate }
		});
}
