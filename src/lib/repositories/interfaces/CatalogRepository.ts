import type { CatalogItem, CatalogItemWithRates } from '$lib/types/index.js';
import type { CreateCatalogItemInput, UpdateCatalogItemInput } from './types.js';

export interface CatalogRepository {
	getCatalogItems(search?: string, category?: string): CatalogItem[];
	getCatalogItem(id: number): CatalogItem | null;
	getCatalogCategories(): string[];
	searchCatalogItems(term: string, limit?: number): CatalogItem[];
	getCatalogItemWithRates(id: number): CatalogItemWithRates | null;
	getCatalogItemsWithTierRate(
		search?: string,
		category?: string,
		tierId?: number
	): (CatalogItem & { tier_rate?: number })[];
	getEffectiveRate(catalogItemId: number, tierId: number | null): number;

	createCatalogItem(data: CreateCatalogItemInput): Promise<number>;
	updateCatalogItem(id: number, data: UpdateCatalogItemInput): Promise<void>;
	deleteCatalogItem(id: number): Promise<void>;
	setCatalogItemRate(catalogItemId: number, tierId: number, rate: number): Promise<void>;

	bulkDeleteCatalogItems(ids: number[]): Promise<void>;
}
