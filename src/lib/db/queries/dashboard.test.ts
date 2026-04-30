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

import { getDashboardStats, getMonthlyRevenue } from './dashboard.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'offset']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getDashboardStats', () => {
	it('is an async function', () => {
		expect(getDashboardStats()).toBeInstanceOf(Promise);
	});

	it('returns dashboard stats object', async () => {
		// Mock the multiple sequential queries that getDashboardStats makes
		// Each call to the chain resolves a different value
		let callCount = 0;
		mockDb.then = (resolve: any) => {
			callCount++;
			switch (callCount) {
				case 1: return resolve([{ total: 5000 }]);     // revenue
				case 2: return resolve([{ total: 2000 }]);     // outstanding
				case 3: return resolve([{ count: 1 }]);        // overdue count
				case 4: return resolve([{ count: 3 }]);        // total clients
				case 5: return resolve([{ count: 10 }]);       // total invoices
				case 6: return resolve([{ count: 0 }]);        // excluded currency
				case 7: return resolve([]);                     // recent invoices
				case 8: return resolve([{ count: 5 }]);        // total estimates
				case 9: return resolve([{ count: 2 }]);        // pending estimates
				case 10: return resolve([]);                    // recent estimates
				default: return resolve([]);
			}
		};

		const stats = await getDashboardStats();
		expect(stats).toHaveProperty('total_revenue');
		expect(stats).toHaveProperty('outstanding_amount');
		expect(stats).toHaveProperty('overdue_count');
		expect(stats).toHaveProperty('total_clients');
		expect(stats).toHaveProperty('total_invoices');
	});
});

describe('getMonthlyRevenue', () => {
	it('is an async function', () => {
		expect(getMonthlyRevenue()).toBeInstanceOf(Promise);
	});

	it('returns 12 months of data', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getMonthlyRevenue();
		expect(result).toHaveLength(12);
		result.forEach((r: any) => {
			expect(r).toHaveProperty('month');
			expect(r).toHaveProperty('label');
			expect(r).toHaveProperty('revenue');
		});
	});

	it('fills missing months with 0 revenue', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getMonthlyRevenue();
		expect(result).toHaveLength(12);
		result.forEach((r: any) => expect(r.revenue).toBe(0));
	});
});
