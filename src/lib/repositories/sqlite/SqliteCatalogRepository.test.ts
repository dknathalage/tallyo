import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/catalog.js', () => ({
	getCatalogItems: vi.fn(),
	getCatalogItem: vi.fn(),
	getCatalogCategories: vi.fn(),
	searchCatalogItems: vi.fn(),
	createCatalogItem: vi.fn(),
	updateCatalogItem: vi.fn(),
	deleteCatalogItem: vi.fn(),
	bulkDeleteCatalogItems: vi.fn(),
	getCatalogItemWithRates: vi.fn(),
	getCatalogItemsWithTierRate: vi.fn(),
	getEffectiveRate: vi.fn(),
	setCatalogItemRate: vi.fn()
}));

vi.mock('$lib/db/audit.js', () => ({
	computeChanges: vi.fn().mockReturnValue({})
}));

import { SqliteCatalogRepository } from './SqliteCatalogRepository.js';
import * as queries from '$lib/db/queries/catalog.js';
import { computeChanges } from '$lib/db/audit.js';
import type { StorageTransaction } from '$lib/repositories/interfaces/StorageTransaction.js';

const mockGetCatalogItems = vi.mocked(queries.getCatalogItems);
const mockGetCatalogItem = vi.mocked(queries.getCatalogItem);
const mockGetCatalogCategories = vi.mocked(queries.getCatalogCategories);
const mockSearchCatalogItems = vi.mocked(queries.searchCatalogItems);
const mockCreateCatalogItem = vi.mocked(queries.createCatalogItem);
const mockUpdateCatalogItem = vi.mocked(queries.updateCatalogItem);
const mockDeleteCatalogItem = vi.mocked(queries.deleteCatalogItem);
const mockBulkDeleteCatalogItems = vi.mocked(queries.bulkDeleteCatalogItems);
const mockGetCatalogItemWithRates = vi.mocked(queries.getCatalogItemWithRates);
const mockGetCatalogItemsWithTierRate = vi.mocked(queries.getCatalogItemsWithTierRate);
const mockGetEffectiveRate = vi.mocked(queries.getEffectiveRate);
const mockSetCatalogItemRate = vi.mocked(queries.setCatalogItemRate);
const mockComputeChanges = vi.mocked(computeChanges);

function makeMockAudit() {
	return { logAudit: vi.fn(), getEntityHistory: vi.fn() };
}

function makeMockTx(): StorageTransaction {
	return {
		run: vi.fn(async (fn: () => Promise<unknown>) => fn()),
		begin: vi.fn(),
		commit: vi.fn(),
		rollback: vi.fn()
	} as unknown as StorageTransaction;
}

beforeEach(() => {
	vi.clearAllMocks();
	mockComputeChanges.mockReturnValue({});
});

describe('SqliteCatalogRepository', () => {
	describe('getCatalogItems', () => {
		it('delegates to getCatalogItems query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			const expected = { data: [], total: 0, page: 1, totalPages: 1 } as any;
			mockGetCatalogItems.mockReturnValue(expected);

			const result = repo.getCatalogItems('search', 'dev');
			expect(mockGetCatalogItems).toHaveBeenCalledWith('search', 'dev', undefined);
			expect(result).toBe(expected);
		});
	});

	describe('getCatalogItem', () => {
		it('delegates to getCatalogItem query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			const item = { id: 1, name: 'Service A' } as any;
			mockGetCatalogItem.mockReturnValue(item);

			expect(repo.getCatalogItem(1)).toBe(item);
			expect(mockGetCatalogItem).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			mockGetCatalogItem.mockReturnValue(null);
			expect(repo.getCatalogItem(999)).toBeNull();
		});
	});

	describe('getCatalogCategories', () => {
		it('delegates to getCatalogCategories query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			mockGetCatalogCategories.mockReturnValue(['dev', 'design']);

			const result = repo.getCatalogCategories();
			expect(result).toEqual(['dev', 'design']);
		});
	});

	describe('searchCatalogItems', () => {
		it('delegates to searchCatalogItems query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			const items = [{ id: 1 }] as any;
			mockSearchCatalogItems.mockReturnValue(items);

			expect(repo.searchCatalogItems('coding', 5)).toBe(items);
			expect(mockSearchCatalogItems).toHaveBeenCalledWith('coding', 5);
		});
	});

	describe('getCatalogItemWithRates', () => {
		it('delegates to getCatalogItemWithRates query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			const item = { id: 1, rates: [] } as any;
			mockGetCatalogItemWithRates.mockReturnValue(item);

			expect(repo.getCatalogItemWithRates(1)).toBe(item);
			expect(mockGetCatalogItemWithRates).toHaveBeenCalledWith(1);
		});
	});

	describe('getCatalogItemsWithTierRate', () => {
		it('delegates to getCatalogItemsWithTierRate query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			const items = [{ id: 1, tier_rate: 100 }] as any;
			mockGetCatalogItemsWithTierRate.mockReturnValue(items);

			const result = repo.getCatalogItemsWithTierRate('search', 'dev', 2);
			expect(mockGetCatalogItemsWithTierRate).toHaveBeenCalledWith('search', 'dev', 2);
			expect(result).toBe(items);
		});
	});

	describe('getEffectiveRate', () => {
		it('delegates to getEffectiveRate query', () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			mockGetEffectiveRate.mockReturnValue(150);

			expect(repo.getEffectiveRate(1, 2)).toBe(150);
			expect(mockGetEffectiveRate).toHaveBeenCalledWith(1, 2);
		});
	});

	describe('createCatalogItem', () => {
		it('calls createCatalogItem and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockCreateCatalogItem.mockResolvedValue(5);

			const data = { name: 'New Item', rate: 100 } as any;
			const id = await repo.createCatalogItem(data);

			expect(mockCreateCatalogItem).toHaveBeenCalledWith(data);
			expect(id).toBe(5);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'catalog', entity_id: 5, action: 'create' })
			);
		});
	});

	describe('updateCatalogItem', () => {
		it('calls updateCatalogItem and logs audit when changes exist', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockGetCatalogItem.mockReturnValue({ id: 1, name: 'Old Item', rate: 100 } as any);
			mockUpdateCatalogItem.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({ rate: { old: 100, new: 150 } });

			await repo.updateCatalogItem(1, { name: 'Old Item', rate: 150 } as any);

			expect(mockUpdateCatalogItem).toHaveBeenCalled();
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'catalog', entity_id: 1, action: 'update' })
			);
		});

		it('does not log audit when no changes', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockGetCatalogItem.mockReturnValue({ id: 1, name: 'Item' } as any);
			mockUpdateCatalogItem.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({});

			await repo.updateCatalogItem(1, { name: 'Item' } as any);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('does not log audit when item not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockGetCatalogItem.mockReturnValue(null);
			mockUpdateCatalogItem.mockResolvedValue(undefined);

			await repo.updateCatalogItem(999, { name: 'Ghost' } as any);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});
	});

	describe('deleteCatalogItem', () => {
		it('calls deleteCatalogItem and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockGetCatalogItem.mockReturnValue({ id: 1, name: 'Service A' } as any);
			mockDeleteCatalogItem.mockResolvedValue(undefined);

			await repo.deleteCatalogItem(1);

			expect(mockDeleteCatalogItem).toHaveBeenCalledWith(1);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'catalog', entity_id: 1, action: 'delete', context: 'Service A' })
			);
		});

		it('uses empty string context when item not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockGetCatalogItem.mockReturnValue(null);
			mockDeleteCatalogItem.mockResolvedValue(undefined);

			await repo.deleteCatalogItem(99);

			expect(audit.logAudit).toHaveBeenCalledWith(expect.objectContaining({ context: '' }));
		});
	});

	describe('setCatalogItemRate', () => {
		it('delegates to setCatalogItemRate query', async () => {
			const repo = new SqliteCatalogRepository(makeMockAudit(), makeMockTx());
			mockSetCatalogItemRate.mockResolvedValue(undefined);

			await repo.setCatalogItemRate(1, 2, 150);
			expect(mockSetCatalogItemRate).toHaveBeenCalledWith(1, 2, 150);
		});
	});

	describe('bulkDeleteCatalogItems', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			await repo.bulkDeleteCatalogItems([]);

			expect(mockBulkDeleteCatalogItems).not.toHaveBeenCalled();
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('calls bulkDeleteCatalogItems and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteCatalogRepository(audit, tx);

			mockGetCatalogItem
				.mockReturnValueOnce({ id: 1, name: 'Item A' } as any)
				.mockReturnValueOnce({ id: 2, name: 'Item B' } as any);
			mockBulkDeleteCatalogItems.mockResolvedValue(undefined);

			await repo.bulkDeleteCatalogItems([1, 2]);

			expect(mockBulkDeleteCatalogItems).toHaveBeenCalledWith([1, 2]);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});
});
