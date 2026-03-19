import { describe, it, expect, vi, beforeEach } from 'vitest';

function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'offset'];
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

vi.mock('./business-profile.js', () => ({
	getBusinessProfile: vi.fn().mockResolvedValue({ default_currency: 'USD' })
}));

import { getClients, getClient, createClient, updateClient, deleteClient, getClientRevenueSummary } from './clients.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'offset']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getClients', () => {
	it('is an async function', () => {
		expect(getClients()).toBeInstanceOf(Promise);
	});

	it('returns paginated result', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getClients();
		expect(result).toHaveProperty('data');
		expect(result).toHaveProperty('total');
		expect(result).toHaveProperty('page');
	});
});

describe('getClient', () => {
	it('is an async function', () => {
		expect(getClient(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getClient(999);
		expect(result).toBeNull();
	});
});

describe('createClient', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 42 }]);
		expect(createClient({ name: 'Bob', email: 'bob@test.com', phone: '555-0100', address: '123 Main St' })).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 42 }]);
		const id = await createClient({ name: 'Bob', email: 'bob@test.com' });
		expect(id).toBe(42);
	});

	it('throws when name is empty string', async () => {
		await expect(createClient({ name: '' })).rejects.toThrow('Client name is required');
	});

	it('throws when name is only whitespace', async () => {
		await expect(createClient({ name: '   ' })).rejects.toThrow('Client name is required');
	});

	it('throws when name is undefined', async () => {
		await expect(createClient({ name: undefined as any })).rejects.toThrow('Client name is required');
	});
});

describe('updateClient', () => {
	it('is an async function', () => {
		expect(updateClient(1, { name: 'Alice Updated', email: 'alice@new.com' })).toBeInstanceOf(Promise);
	});

	it('throws when name is empty string', async () => {
		await expect(updateClient(1, { name: '' })).rejects.toThrow('Client name is required');
	});

	it('throws when name is only whitespace', async () => {
		await expect(updateClient(1, { name: '  ' })).rejects.toThrow('Client name is required');
	});
});

describe('deleteClient', () => {
	it('is an async function', () => {
		expect(deleteClient(5)).toBeInstanceOf(Promise);
	});

	it('propagates errors', async () => {
		mockDb.where.mockRejectedValueOnce(new Error('DELETE failed'));
		await expect(deleteClient(5)).rejects.toThrow('DELETE failed');
	});
});

describe('getClientRevenueSummary', () => {
	it('is an async function', () => {
		mockDb.then = (resolve: any) => resolve([{ total_invoiced: 0, total_paid: 0, outstanding_balance: 0, invoice_count: 0 }]);
		expect(getClientRevenueSummary(1)).toBeInstanceOf(Promise);
	});
});
