import { describe, it, expect, vi, beforeEach } from 'vitest';

function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy'];
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
	getRateTiers,
	createRateTier,
	updateRateTier,
	deleteRateTier,
	getDefaultTier
} from './rate-tiers.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getRateTiers', () => {
	it('is an async function', () => {
		expect(getRateTiers()).toBeInstanceOf(Promise);
	});

	it('returns empty array when no tiers exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getRateTiers();
		expect(result).toEqual([]);
	});
});

describe('getDefaultTier', () => {
	it('is an async function', () => {
		expect(getDefaultTier()).toBeInstanceOf(Promise);
	});

	it('returns null when no tiers exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getDefaultTier();
		expect(result).toBeNull();
	});
});

describe('createRateTier', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 4 }]);
		expect(createRateTier({ name: 'VIP', description: 'VIP clients', sort_order: 2 })).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 4 }]);
		const id = await createRateTier({ name: 'VIP' });
		expect(id).toBe(4);
	});

	it('throws when name is empty', async () => {
		await expect(createRateTier({ name: '' })).rejects.toThrow('Tier name is required');
	});

	it('throws when name is only whitespace', async () => {
		await expect(createRateTier({ name: '   ' })).rejects.toThrow('Tier name is required');
	});
});

describe('updateRateTier', () => {
	it('is an async function', () => {
		expect(updateRateTier(2, { name: 'Premium Plus', description: 'Top tier', sort_order: 3 })).toBeInstanceOf(Promise);
	});

	it('throws when new name is empty', async () => {
		await expect(updateRateTier(1, { name: '' })).rejects.toThrow('Tier name is required');
	});
});

describe('deleteRateTier', () => {
	it('is an async function', () => {
		// Mock that there are 2 tiers so deletion is allowed
		mockDb.then = (resolve: any) => resolve([{ id: 1 }, { id: 2 }]);
		expect(deleteRateTier(2)).toBeInstanceOf(Promise);
	});

	it('throws when trying to delete the last tier', async () => {
		// deleteRateTier first does a count query: select({ count: ... }).from(...)
		// which resolves to [{ count: 1 }]
		mockDb.then = (resolve: any) => resolve([{ count: 1 }]);
		await expect(deleteRateTier(1)).rejects.toThrow('Cannot delete the last tier');
	});
});
