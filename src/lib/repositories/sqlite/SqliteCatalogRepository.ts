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
import { save } from '$lib/db/connection.svelte.js';
import { computeChanges } from '$lib/db/audit.js';
import type { CatalogRepository } from '../interfaces/CatalogRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';
import type { CreateCatalogItemInput, UpdateCatalogItemInput } from '../interfaces/types.js';
import type { CatalogItem, CatalogItemWithRates } from '$lib/types/index.js';

export class SqliteCatalogRepository implements CatalogRepository {
	constructor(
		private readonly _audit: AuditRepository,
		private readonly _tx: StorageTransaction
	) {}

	getCatalogItems(search?: string, category?: string): CatalogItem[] {
		return getCatalogItems(search, category);
	}

	getCatalogItem(id: number): CatalogItem | null {
		return getCatalogItem(id);
	}

	getCatalogCategories(): string[] {
		return getCatalogCategories();
	}

	searchCatalogItems(term: string, limit?: number): CatalogItem[] {
		return searchCatalogItems(term, limit);
	}

	getCatalogItemWithRates(id: number): CatalogItemWithRates | null {
		return getCatalogItemWithRates(id);
	}

	getCatalogItemsWithTierRate(
		search?: string,
		category?: string,
		tierId?: number
	): (CatalogItem & { tier_rate?: number })[] {
		return getCatalogItemsWithTierRate(search, category, tierId);
	}

	getEffectiveRate(catalogItemId: number, tierId: number | null): number {
		return getEffectiveRate(catalogItemId, tierId);
	}

	async createCatalogItem(data: CreateCatalogItemInput): Promise<number> {
		const id = await createCatalogItem(data);
		this._audit.logAudit({
			entity_type: 'catalog',
			entity_id: id,
			action: 'create',
			changes: {
				name: { old: null, new: data.name },
				rate: { old: null, new: data.rate ?? 0 }
			}
		});
		await save();
		return id;
	}

	async updateCatalogItem(id: number, data: UpdateCatalogItemInput): Promise<void> {
		const oldItem = getCatalogItem(id);
		await updateCatalogItem(id, data);
		if (oldItem) {
			const changes = computeChanges(
				oldItem as unknown as Record<string, unknown>,
				{ name: data.name, rate: data.rate ?? 0, unit: data.unit ?? '', category: data.category ?? '', sku: data.sku ?? '' },
				['name', 'rate', 'unit', 'category', 'sku']
			);
			if (Object.keys(changes).length > 0) {
				this._audit.logAudit({ entity_type: 'catalog', entity_id: id, action: 'update', changes });
			}
		}
		await save();
	}

	async deleteCatalogItem(id: number): Promise<void> {
		const item = getCatalogItem(id);
		await deleteCatalogItem(id);
		this._audit.logAudit({ entity_type: 'catalog', entity_id: id, action: 'delete', context: item?.name ?? '' });
		await save();
	}

	async setCatalogItemRate(catalogItemId: number, tierId: number, rate: number): Promise<void> {
		await setCatalogItemRate(catalogItemId, tierId, rate);
		await save();
	}

	async bulkDeleteCatalogItems(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const items = ids.map((id) => getCatalogItem(id));
		await bulkDeleteCatalogItems(ids);
		for (let i = 0; i < ids.length; i++) {
			this._audit.logAudit({ entity_type: 'catalog', entity_id: ids[i], action: 'delete', context: items[i]?.name ?? '', batch_id });
		}
		await save();
	}
}
