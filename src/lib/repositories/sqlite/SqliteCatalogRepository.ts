import {
	getCatalogItems,
	getCatalogItem,
	getCatalogCategories,
	searchCatalogItems,
	createCatalogItem,
	updateCatalogItem,
	deleteCatalogItem,
	bulkDeleteCatalogItems,
	getCatalogItemWithRates,
	getCatalogItemsWithTierRate,
	getEffectiveRate,
	setCatalogItemRate
} from '$lib/db/queries/catalog.js';

import { computeChanges } from '$lib/db/audit.js';
import type { CatalogRepository } from '../interfaces/CatalogRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { CreateCatalogItemInput, UpdateCatalogItemInput } from '../interfaces/types.js';
import type { CatalogItem, CatalogItemWithRates, PaginationParams, PaginatedResult } from '$lib/types/index.js';

export class SqliteCatalogRepository implements CatalogRepository {
	constructor(private readonly _audit: AuditRepository) {}

	async getCatalogItems(search?: string, category?: string, pagination?: PaginationParams): Promise<PaginatedResult<CatalogItem>> {
		return await getCatalogItems(search, category, pagination);
	}

	async getCatalogItem(id: number): Promise<CatalogItem | null> {
		return await getCatalogItem(id);
	}

	async getCatalogCategories(): Promise<string[]> {
		return await getCatalogCategories();
	}

	async searchCatalogItems(term: string, limit?: number): Promise<CatalogItem[]> {
		return await searchCatalogItems(term, limit);
	}

	async getCatalogItemWithRates(id: number): Promise<CatalogItemWithRates | null> {
		return await getCatalogItemWithRates(id);
	}

	async getCatalogItemsWithTierRate(
		search?: string,
		category?: string,
		tierId?: number
	): Promise<(CatalogItem & { tier_rate?: number })[]> {
		return await getCatalogItemsWithTierRate(search, category, tierId);
	}

	async getEffectiveRate(catalogItemId: number, tierId: number | null): Promise<number> {
		return await getEffectiveRate(catalogItemId, tierId);
	}

	async createCatalogItem(data: CreateCatalogItemInput): Promise<number> {
		const id = await createCatalogItem(data);
		await this._audit.logAudit({
			entity_type: 'catalog',
			entity_id: id,
			action: 'create',
			changes: {
				name: { old: null, new: data.name },
				rate: { old: null, new: data.rate ?? 0 }
			}
		});
		return id;
	}

	async updateCatalogItem(id: number, data: UpdateCatalogItemInput): Promise<void> {
		const oldItem = await getCatalogItem(id);
		await updateCatalogItem(id, data);
		if (oldItem) {
			const changes = computeChanges(
				oldItem as unknown as Record<string, unknown>,
				{ name: data.name, rate: data.rate ?? 0, unit: data.unit ?? '', category: data.category ?? '', sku: data.sku ?? '' },
				['name', 'rate', 'unit', 'category', 'sku']
			);
			if (Object.keys(changes).length > 0) {
				await this._audit.logAudit({ entity_type: 'catalog', entity_id: id, action: 'update', changes });
			}
		}
	}

	async deleteCatalogItem(id: number): Promise<void> {
		const item = await getCatalogItem(id);
		await deleteCatalogItem(id);
		await this._audit.logAudit({ entity_type: 'catalog', entity_id: id, action: 'delete', context: item?.name ?? '' });
	}

	async setCatalogItemRate(catalogItemId: number, tierId: number, rate: number): Promise<void> {
		await setCatalogItemRate(catalogItemId, tierId, rate);
	}

	async bulkDeleteCatalogItems(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const items = await Promise.all(ids.map((id) => getCatalogItem(id)));
		await bulkDeleteCatalogItems(ids);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({ entity_type: 'catalog', entity_id: ids[i], action: 'delete', context: items[i]?.name ?? '', batch_id });
		}
	}
}
