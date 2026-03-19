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
	getTaxRates,
	getDefaultTaxRate,
	createTaxRate,
	updateTaxRate,
	deleteTaxRate
} from './tax-rates.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getTaxRates', () => {
	it('is an async function', () => {
		expect(getTaxRates()).toBeInstanceOf(Promise);
	});

	it('returns empty array when no tax rates exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getTaxRates();
		expect(result).toEqual([]);
	});
});

describe('getDefaultTaxRate', () => {
	it('is an async function', () => {
		expect(getDefaultTaxRate()).toBeInstanceOf(Promise);
	});

	it('returns null when no default tax rate is set', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getDefaultTaxRate();
		expect(result).toBeNull();
	});
});

describe('createTaxRate', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 2 }]);
		expect(createTaxRate({ name: 'VAT', rate: 20 })).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 2 }]);
		const id = await createTaxRate({ name: 'VAT', rate: 20 });
		expect(id).toBe(2);
	});
});

describe('updateTaxRate', () => {
	it('is an async function', () => {
		expect(updateTaxRate(1, { name: 'GST Updated', rate: 11 })).toBeInstanceOf(Promise);
	});
});

describe('deleteTaxRate', () => {
	it('is an async function', () => {
		expect(deleteTaxRate(3)).toBeInstanceOf(Promise);
	});
});
