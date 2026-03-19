import type { CatalogItem, CatalogItemWithRates, PaginationParams, PaginatedResult } from '$lib/types/index.js';
import type { CreateCatalogItemInput, UpdateCatalogItemInput } from './types.js';

export interface CatalogRepository {
	getCatalogItems(search?: string, category?: string, pagination?: PaginationParams): Promise<PaginatedResult<CatalogItem>>;
	getCatalogItem(id: number): Promise<CatalogItem | null>;
	getCatalogCategories(): Promise<string[]>;
	searchCatalogItems(term: string, limit?: number): Promise<CatalogItem[]>;
	getCatalogItemWithRates(id: number): Promise<CatalogItemWithRates | null>;
	getCatalogItemsWithTierRate(
		search?: string,
		category?: string,
		tierId?: number
	): Promise<(CatalogItem & { tier_rate?: number })[]>;
	getEffectiveRate(catalogItemId: number, tierId: number | null): Promise<number>;

	createCatalogItem(data: CreateCatalogItemInput): Promise<number>;
	updateCatalogItem(id: number, data: UpdateCatalogItemInput): Promise<void>;
	deleteCatalogItem(id: number): Promise<void>;
	setCatalogItemRate(catalogItemId: number, tierId: number, rate: number): Promise<void>;

	bulkDeleteCatalogItems(ids: number[]): Promise<void>;
}
