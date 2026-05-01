import { getDb } from '../connection.js';
import { catalogItems, catalogItemRates } from '../drizzle-schema.js';
import { eq, like, or, and, ne, inArray, asc } from 'drizzle-orm';
import type {
	CatalogItem,
	CatalogItemWithRates,
	PaginationParams,
	PaginatedResult
} from '../../types/index.js';
import { paginate } from '../../types/index.js';

function mapRow(row: Record<string, unknown>): CatalogItem {
	return {
		id: row['id'] as number,
		uuid: row['uuid'] as string,
		name: row['name'] as string,
		rate: (row['rate'] as number | null | undefined) ?? 0,
		unit: (row['unit'] as string | null | undefined) ?? '',
		category: (row['category'] as string | null | undefined) ?? '',
		sku: (row['sku'] as string | null | undefined) ?? '',
		metadata: (row['metadata'] as string | null | undefined) ?? '{}',
		created_at: (row['created_at'] as string | null | undefined) ?? '',
		updated_at: (row['updated_at'] as string | null | undefined) ?? ''
	};
}

export async function getCatalogItems(
	search?: string,
	category?: string,
	pagination?: PaginationParams
): Promise<PaginatedResult<CatalogItem>> {
	const db = getDb();
	const conditions = buildCatalogConditions(search, category);
	const base = db.select().from(catalogItems);
	const filtered = conditions.length > 0 ? base.where(and(...conditions)) : base;
	const rows = await filtered.orderBy(asc(catalogItems.name));
	const all = rows.map(mapRow);
	return paginate(all, pagination);
}

export async function getCatalogItem(id: number): Promise<CatalogItem | null> {
	const db = getDb();
	const rows = await db.select().from(catalogItems).where(eq(catalogItems.id, id));
	const first = rows[0];
	return first ? mapRow(first) : null;
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
	limit = 10
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
	const name = data.name as string | null | undefined;
	if (!name?.trim()) {
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

	const inserted = result[0];
	if (!inserted) throw new Error('Failed to insert catalog item');
	return inserted.id;
}

/**
 * Updates a catalog item.
 */
export async function updateCatalogItem(
	id: number,
	data: { name: string; rate?: number; unit?: string; category?: string; sku?: string }
): Promise<void> {
	const name = data.name as string | null | undefined;
	if (!name?.trim()) {
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
			updated_at: new Date().toISOString()
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

	const item = rows[0];
	if (!item) return null;
	const rates: Record<number, number> = {};
	for (const row of rows) {
		if (row.tier_id !== null && row.tier_rate !== null) {
			rates[row.tier_id] = row.tier_rate;
		}
	}

	return {
		id: item.id,
		uuid: item.uuid ?? '',
		name: item.name,
		rate: item.rate,
		unit: item.unit ?? '',
		category: item.category ?? '',
		sku: item.sku ?? '',
		metadata: item.metadata ?? '{}',
		created_at: item.created_at ?? '',
		updated_at: item.updated_at ?? '',
		rates
	};
}

function buildCatalogConditions(search?: string, category?: string) {
	const conditions = [];
	if (search) {
		const clause = or(
			like(catalogItems.name, `%${search}%`),
			like(catalogItems.sku, `%${search}%`)
		);
		if (clause) conditions.push(clause);
	}
	if (category) {
		conditions.push(eq(catalogItems.category, category));
	}
	return conditions;
}

const catalogBaseSelect = {
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
};

export async function getCatalogItemsWithTierRate(
	search?: string,
	category?: string,
	tierId?: number
): Promise<(CatalogItem & { tier_rate?: number })[]> {
	const db = getDb();
	const conditions = buildCatalogConditions(search, category);
	const rows = tierId
		? await runTieredQuery(db, conditions, tierId)
		: await runUntieredQuery(db, conditions);

	return rows.map((row) => {
		const tierRate = 'tier_rate' in row ? (row.tier_rate as number | null | undefined) : undefined;
		return {
			...mapRow(row),
			...(tierRate !== undefined && tierRate !== null && { tier_rate: tierRate })
		};
	});
}

type Db = ReturnType<typeof getDb>;
type Conditions = ReturnType<typeof buildCatalogConditions>;

async function runTieredQuery(db: Db, conditions: Conditions, tierId: number) {
	const baseQuery = db
		.select({ ...catalogBaseSelect, tier_rate: catalogItemRates.rate })
		.from(catalogItems)
		.leftJoin(
			catalogItemRates,
			and(
				eq(catalogItems.id, catalogItemRates.catalog_item_id),
				eq(catalogItemRates.rate_tier_id, tierId)
			)
		);
	const filtered = conditions.length > 0 ? baseQuery.where(and(...conditions)) : baseQuery;
	return filtered.orderBy(asc(catalogItems.name));
}

async function runUntieredQuery(db: Db, conditions: Conditions) {
	const baseQuery = db.select(catalogBaseSelect).from(catalogItems);
	const filtered = conditions.length > 0 ? baseQuery.where(and(...conditions)) : baseQuery;
	return filtered.orderBy(asc(catalogItems.name));
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
		const firstRate = results[0];
		if (firstRate) {
			return firstRate.rate;
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
