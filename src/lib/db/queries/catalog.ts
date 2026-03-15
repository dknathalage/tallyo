import { execute, query } from '../connection.js';
import type { CatalogItem, CatalogItemWithRates, CatalogItemRate } from '../../types/index.js';

export function getCatalogItems(search?: string, category?: string): CatalogItem[] {
	if (search && category) {
		return query<CatalogItem>(
			`SELECT * FROM catalog_items WHERE (name LIKE ? OR sku LIKE ?) AND category = ? ORDER BY name`,
			[`%${search}%`, `%${search}%`, category]
		);
	}
	if (search) {
		return query<CatalogItem>(
			`SELECT * FROM catalog_items WHERE name LIKE ? OR sku LIKE ? ORDER BY name`,
			[`%${search}%`, `%${search}%`]
		);
	}
	if (category) {
		return query<CatalogItem>(
			`SELECT * FROM catalog_items WHERE category = ? ORDER BY name`,
			[category]
		);
	}
	return query<CatalogItem>(`SELECT * FROM catalog_items ORDER BY name`);
}

export function getCatalogItem(id: number): CatalogItem | null {
	const results = query<CatalogItem>(`SELECT * FROM catalog_items WHERE id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

export function getCatalogCategories(): string[] {
	const results = query<{ category: string }>(
		`SELECT DISTINCT category FROM catalog_items WHERE category != '' ORDER BY category`
	);
	return results.map((r) => r.category);
}

export function searchCatalogItems(term: string, limit: number = 10): CatalogItem[] {
	return query<CatalogItem>(
		`SELECT * FROM catalog_items WHERE name LIKE ? OR sku LIKE ? ORDER BY name LIMIT ?`,
		[`%${term}%`, `%${term}%`, limit]
	);
}

/**
 * Pure SQL: inserts a catalog item and returns the new id.
 * No audit logging, no save().
 */
export function createCatalogItem(data: {
	name: string;
	rate?: number;
	unit?: string;
	category?: string;
	sku?: string;
}): number {
	if (!data.name?.trim()) {
		throw new Error('Catalog item name is required');
	}
	execute(
		`INSERT INTO catalog_items (uuid, name, rate, unit, category, sku) VALUES (?, ?, ?, ?, ?, ?)`,
		[crypto.randomUUID(), data.name, data.rate ?? 0, data.unit ?? '', data.category ?? '', data.sku ?? '']
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	return result[0].id;
}

/**
 * Pure SQL: updates a catalog item.
 * No audit logging, no save().
 */
export function updateCatalogItem(
	id: number,
	data: { name: string; rate?: number; unit?: string; category?: string; sku?: string }
): void {
	if (!data.name?.trim()) {
		throw new Error('Catalog item name is required');
	}
	execute(
		`UPDATE catalog_items SET name = ?, rate = ?, unit = ?, category = ?, sku = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.rate ?? 0, data.unit ?? '', data.category ?? '', data.sku ?? '', id]
	);
}

/**
 * Pure SQL: deletes a catalog item.
 * No audit logging, no save().
 */
export function deleteCatalogItem(id: number): void {
	execute(`DELETE FROM catalog_items WHERE id = ?`, [id]);
}

/**
 * Pure SQL: bulk deletes catalog items.
 * No audit logging, no save().
 */
export function bulkDeleteCatalogItems(ids: number[]): void {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM catalog_items WHERE id IN (${placeholders})`, ids);
}

export function getCatalogItemWithRates(id: number): CatalogItemWithRates | null {
	const item = getCatalogItem(id);
	if (!item) return null;

	const rateRows = query<CatalogItemRate>(
		`SELECT * FROM catalog_item_rates WHERE catalog_item_id = ?`,
		[id]
	);

	const rates: Record<number, number> = {};
	for (const row of rateRows) {
		rates[row.rate_tier_id] = row.rate;
	}

	return { ...item, rates };
}

export function getCatalogItemsWithTierRate(
	search?: string,
	category?: string,
	tierId?: number
): (CatalogItem & { tier_rate?: number })[] {
	let sql: string;
	const params: unknown[] = [];

	if (tierId) {
		sql = `SELECT ci.*, cir.rate as tier_rate FROM catalog_items ci LEFT JOIN catalog_item_rates cir ON ci.id = cir.catalog_item_id AND cir.rate_tier_id = ?`;
		params.push(tierId);
	} else {
		sql = `SELECT * FROM catalog_items ci`;
	}

	const conditions: string[] = [];
	if (search) {
		conditions.push(`(ci.name LIKE ? OR ci.sku LIKE ?)`);
		params.push(`%${search}%`, `%${search}%`);
	}
	if (category) {
		conditions.push(`ci.category = ?`);
		params.push(category);
	}

	if (conditions.length > 0) {
		sql += ` WHERE ${conditions.join(' AND ')}`;
	}

	sql += ` ORDER BY ci.name`;

	return query<CatalogItem & { tier_rate?: number }>(sql, params);
}

export function getEffectiveRate(catalogItemId: number, tierId: number | null): number {
	if (tierId) {
		const results = query<{ rate: number }>(
			`SELECT rate FROM catalog_item_rates WHERE catalog_item_id = ? AND rate_tier_id = ?`,
			[catalogItemId, tierId]
		);
		if (results.length > 0) {
			return results[0].rate;
		}
	}
	const item = getCatalogItem(catalogItemId);
	return item?.rate ?? 0;
}

/**
 * Pure SQL: sets a catalog item rate for a tier.
 * No save().
 */
export function setCatalogItemRate(
	catalogItemId: number,
	tierId: number,
	rate: number
): void {
	execute(
		`INSERT OR REPLACE INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate) VALUES (?, ?, ?)`,
		[catalogItemId, tierId, rate]
	);
}
