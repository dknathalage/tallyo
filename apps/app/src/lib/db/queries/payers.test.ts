import { describe, it, expect, vi, beforeEach } from 'vitest';

function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', '$dynamic'];
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

import { getPayers, getPayer, createPayer, updatePayer, deletePayer, buildPayerSnapshot } from './payers.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', '$dynamic']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getPayers', () => {
	it('is an async function', () => {
		expect(getPayers()).toBeInstanceOf(Promise);
	});

	it('returns empty array when no payers', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getPayers();
		expect(result).toEqual([]);
	});
});

describe('getPayer', () => {
	it('is an async function', () => {
		expect(getPayer(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getPayer(999);
		expect(result).toBeNull();
	});
});

describe('createPayer', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 10 }]);
		expect(createPayer({ name: 'New Payer', email: 'payer@test.com' })).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 10 }]);
		const id = await createPayer({ name: 'New Payer', email: 'payer@test.com' });
		expect(id).toBe(10);
	});

	it('throws when name is empty', async () => {
		await expect(createPayer({ name: '' })).rejects.toThrow('Payer name is required');
	});

	it('throws when name is whitespace', async () => {
		await expect(createPayer({ name: '   ' })).rejects.toThrow('Payer name is required');
	});
});

describe('updatePayer', () => {
	it('is an async function', () => {
		expect(updatePayer(1, { name: 'Updated Payer', email: 'new@test.com' })).toBeInstanceOf(Promise);
	});

	it('throws when name is empty', async () => {
		await expect(updatePayer(1, { name: '' })).rejects.toThrow('Payer name is required');
	});
});

describe('deletePayer', () => {
	it('is an async function', () => {
		expect(deletePayer(5)).toBeInstanceOf(Promise);
	});
});

describe('buildPayerSnapshot', () => {
	it('is an async function', () => {
		expect(buildPayerSnapshot(null)).toBeInstanceOf(Promise);
	});

	it('returns empty snapshot for null payerId', async () => {
		const result = await buildPayerSnapshot(null);
		expect(result).toEqual({
			name: '',
			email: '',
			phone: '',
			address: '',
			metadata: {}
		});
	});

	it('returns empty snapshot when payer not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await buildPayerSnapshot(999);
		expect(result).toEqual({
			name: '',
			email: '',
			phone: '',
			address: '',
			metadata: {}
		});
	});
});
