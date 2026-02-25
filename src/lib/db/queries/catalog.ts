import { execute, query, save } from '../connection.svelte.js';
import { logAudit, computeChanges } from '../audit.js';
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
	execute(
		`INSERT INTO catalog_items (uuid, name, rate, unit, category, sku) VALUES (?, ?, ?, ?, ?, ?)`,
		[crypto.randomUUID(), data.name, data.rate ?? 0, data.unit ?? '', data.category ?? '', data.sku ?? '']
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	logAudit({
		entity_type: 'catalog',
		entity_id: result[0].id,
		action: 'create',
		changes: {
			name: { old: null, new: data.name },
			rate: { old: null, new: data.rate ?? 0 }
		}
	});
	await save();
	return result[0].id;
}

export async function updateCatalogItem(
	id: number,
	data: { name: string; rate?: number; unit?: string; category?: string; sku?: string }
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Catalog item name is required');
	}
	const oldItem = getCatalogItem(id);
	execute(
		`UPDATE catalog_items SET name = ?, rate = ?, unit = ?, category = ?, sku = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.rate ?? 0, data.unit ?? '', data.category ?? '', data.sku ?? '', id]
	);
	if (oldItem) {
		const changes = computeChanges(
			oldItem as unknown as Record<string, unknown>,
			{ name: data.name, rate: data.rate ?? 0, unit: data.unit ?? '', category: data.category ?? '', sku: data.sku ?? '' },
			['name', 'rate', 'unit', 'category', 'sku']
		);
		if (Object.keys(changes).length > 0) {
			logAudit({ entity_type: 'catalog', entity_id: id, action: 'update', changes });
		}
	}
	await save();
}

export async function deleteCatalogItem(id: number): Promise<void> {
	const item = getCatalogItem(id);
	execute(`DELETE FROM catalog_items WHERE id = ?`, [id]);
	logAudit({ entity_type: 'catalog', entity_id: id, action: 'delete', context: item?.name ?? '' });
	await save();
}

export async function bulkDeleteCatalogItems(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const batch_id = crypto.randomUUID();
	const items = ids.map((id) => getCatalogItem(id));
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM catalog_items WHERE id IN (${placeholders})`, ids);
	for (let i = 0; i < ids.length; i++) {
		logAudit({ entity_type: 'catalog', entity_id: ids[i], action: 'delete', context: items[i]?.name ?? '', batch_id });
	}
	await save();
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

export async function setCatalogItemRate(
	catalogItemId: number,
	tierId: number,
	rate: number
): Promise<void> {
	execute(
		`INSERT OR REPLACE INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate) VALUES (?, ?, ?)`,
		[catalogItemId, tierId, rate]
	);
	await save();
}
