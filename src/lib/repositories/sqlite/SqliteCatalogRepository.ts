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
import type { CatalogRepository } from '../interfaces/CatalogRepository.js';
import type { CreateCatalogItemInput, UpdateCatalogItemInput } from '../interfaces/types.js';
import type { CatalogItem, CatalogItemWithRates } from '$lib/types/index.js';

export class SqliteCatalogRepository implements CatalogRepository {
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

	createCatalogItem(data: CreateCatalogItemInput): Promise<number> {
		return createCatalogItem(data);
	}

	updateCatalogItem(id: number, data: UpdateCatalogItemInput): Promise<void> {
		return updateCatalogItem(id, data);
	}

	deleteCatalogItem(id: number): Promise<void> {
		return deleteCatalogItem(id);
	}

	setCatalogItemRate(catalogItemId: number, tierId: number, rate: number): Promise<void> {
		return setCatalogItemRate(catalogItemId, tierId, rate);
	}

	bulkDeleteCatalogItems(ids: number[]): Promise<void> {
		return bulkDeleteCatalogItems(ids);
	}
}
