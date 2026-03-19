import { describe, it, expect, vi, beforeEach } from 'vitest';

function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'offset', 'innerJoin'];
	for (const m of methods) {
		chain[m] = vi.fn().mockReturnValue(chain);
	}
	chain.then = (resolve: any) => resolve([]);
	chain[Symbol.iterator] = function* () {};
	chain.transaction = vi.fn(async (fn: any) => fn(chain));
	return chain;
}

const mockDb = createMockDb();

vi.mock('../connection.js', () => ({
	getDb: vi.fn(() => mockDb)
}));

import {
	getCatalogItems,
	searchCatalogItems,
	createCatalogItem,
	updateCatalogItem,
	deleteCatalogItem
} from './catalog.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'offset', 'innerJoin']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getCatalogItems', () => {
	it('is an async function', () => {
		expect(getCatalogItems()).toBeInstanceOf(Promise);
	});

	it('returns paginated result', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getCatalogItems();
		expect(result).toHaveProperty('data');
		expect(result).toHaveProperty('total');
	});
});

describe('searchCatalogItems', () => {
	it('is an async function', () => {
		expect(searchCatalogItems('bolt')).toBeInstanceOf(Promise);
	});

	it('returns empty array when no results match', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await searchCatalogItems('nonexistent-xyz');
		expect(result).toEqual([]);
	});
});

describe('createCatalogItem', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 3 }]);
		expect(createCatalogItem({ name: 'Widget Pro', rate: 25 })).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 3 }]);
		const id = await createCatalogItem({ name: 'Widget Pro', rate: 25 });
		expect(id).toBe(3);
	});

	it('throws when name is empty', async () => {
		await expect(createCatalogItem({ name: '' })).rejects.toThrow(
			'Catalog item name is required'
		);
	});

	it('throws when name is only whitespace', async () => {
		await expect(createCatalogItem({ name: '   ' })).rejects.toThrow(
			'Catalog item name is required'
		);
	});
});

describe('updateCatalogItem', () => {
	it('is an async function', () => {
		expect(updateCatalogItem(2, { name: 'New Name', rate: 20 })).toBeInstanceOf(Promise);
	});

	it('throws when new name is empty', async () => {
		await expect(updateCatalogItem(1, { name: '' })).rejects.toThrow(
			'Catalog item name is required'
		);
	});
});

describe('deleteCatalogItem', () => {
	it('is an async function', () => {
		expect(deleteCatalogItem(5)).toBeInstanceOf(Promise);
	});

	it('propagates errors', async () => {
		mockDb.where.mockRejectedValueOnce(new Error('DELETE failed'));
		await expect(deleteCatalogItem(5)).rejects.toThrow('DELETE failed');
	});
});
