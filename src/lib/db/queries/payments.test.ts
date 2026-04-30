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
	getInvoicePayments,
	getInvoiceTotalPaid,
	createPayment,
	deletePayment
} from './payments.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getInvoicePayments', () => {
	it('is an async function', () => {
		expect(getInvoicePayments(1)).toBeInstanceOf(Promise);
	});

	it('returns empty array when no payments exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getInvoicePayments(99);
		expect(result).toEqual([]);
	});
});

describe('getInvoiceTotalPaid', () => {
	it('is an async function', () => {
		mockDb.then = (resolve: any) => resolve([{ total: 150 }]);
		expect(getInvoiceTotalPaid(1)).toBeInstanceOf(Promise);
	});

	it('returns the sum of payment amounts', async () => {
		mockDb.then = (resolve: any) => resolve([{ total: 150 }]);
		const result = await getInvoiceTotalPaid(1);
		expect(result).toBe(150);
	});

	it('returns 0 when no payments exist (null total)', async () => {
		mockDb.then = (resolve: any) => resolve([{ total: null }]);
		const result = await getInvoiceTotalPaid(1);
		expect(result).toBe(0);
	});

	it('returns 0 when query returns empty array', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getInvoiceTotalPaid(1);
		expect(result).toBe(0);
	});
});

describe('createPayment', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 5 }]);
		expect(createPayment({
			invoice_id: 1,
			amount: 100,
			payment_date: '2025-01-15',
			method: 'bank',
			notes: 'Partial payment'
		})).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 5 }]);
		const id = await createPayment({
			invoice_id: 1,
			amount: 100,
			payment_date: '2025-01-15'
		});
		expect(id).toBe(5);
	});
});

describe('deletePayment', () => {
	it('is an async function', () => {
		expect(deletePayment(4)).toBeInstanceOf(Promise);
	});
});
