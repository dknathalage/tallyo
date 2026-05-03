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
	chain.all = vi.fn(() => [{ id: 1 }]);
	chain.run = vi.fn(() => undefined);
	chain.get = vi.fn(() => ({ id: 1 }));
	chain.transaction = vi.fn((fn: any) => fn(chain));
	return chain;
}

const mockDb = createMockDb();

vi.mock('../connection.js', () => ({
	getDb: vi.fn(() => mockDb)
}));

vi.mock('../number-generators.js', () => ({
	generateInvoiceNumber: vi.fn().mockResolvedValue('INV-0001'),
	generateEstimateNumber: vi.fn().mockResolvedValue('EST-0001')
}));

import {
	getEstimates,
	getEstimate,
	getEstimateLineItems,
	createEstimate,
	updateEstimate,
	deleteEstimate,
	updateEstimateStatus,
	getClientEstimates,
	bulkDeleteEstimates,
	bulkUpdateEstimateStatus,
	convertEstimateToInvoice
} from './estimates.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'offset', 'innerJoin']) {
		mockDb[m].mockReturnValue(mockDb);
	}
	mockDb.all.mockReturnValue([{ id: 1 }]);
	mockDb.run.mockReturnValue(undefined);
	mockDb.get.mockReturnValue({ id: 1 });
	mockDb.transaction.mockImplementation((fn: any) => fn(mockDb));
});

describe('getEstimates', () => {
	it('is an async function', () => {
		expect(getEstimates()).toBeInstanceOf(Promise);
	});

	it('returns paginated result', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getEstimates();
		expect(result).toHaveProperty('data');
		expect(result).toHaveProperty('total');
	});
});

describe('getEstimate', () => {
	it('is an async function', () => {
		expect(getEstimate(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getEstimate(999);
		expect(result).toBeNull();
	});
});

describe('getEstimateLineItems', () => {
	it('is an async function', () => {
		expect(getEstimateLineItems(1)).toBeInstanceOf(Promise);
	});
});

describe('createEstimate', () => {
	it('is an async function', () => {
		mockDb.returning.mockReturnValue(mockDb);
		mockDb.all.mockReturnValue([{ id: 7 }]);
		const lineItems = [
			{ description: 'Service A', quantity: 1, rate: 100, amount: 100, sort_order: 0, notes: 'Test note' }
		];
		expect(createEstimate({
			estimate_number: 'EST-0001',
			client_id: 1,
			date: '2025-01-01',
			valid_until: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110
		}, lineItems)).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockReturnValue(mockDb);
		mockDb.all.mockReturnValue([{ id: 7 }]);
		const id = await createEstimate({
			estimate_number: 'EST-0001',
			client_id: 1,
			date: '2025-01-01',
			valid_until: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110
		}, []);
		expect(id).toBe(7);
	});

	it('propagates errors', async () => {
		mockDb.transaction.mockRejectedValueOnce(new Error('SQL error'));
		await expect(createEstimate({
			estimate_number: 'EST-0001',
			client_id: 1,
			date: '2025-01-01',
			valid_until: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110
		}, [])).rejects.toThrow('SQL error');
	});
});

describe('updateEstimate', () => {
	it('is an async function', () => {
		expect(updateEstimate(1, {
			estimate_number: 'EST-0001',
			client_id: 1,
			date: '2025-01-01',
			valid_until: '2025-02-01',
			subtotal: 200,
			tax_rate: 10,
			tax_amount: 20,
			total: 220,
			notes: 'Updated',
			status: 'sent'
		}, [])).toBeInstanceOf(Promise);
	});
});

describe('deleteEstimate', () => {
	it('is an async function', () => {
		expect(deleteEstimate(3)).toBeInstanceOf(Promise);
	});

	it('propagates errors', async () => {
		mockDb.where.mockRejectedValueOnce(new Error('DELETE failed'));
		await expect(deleteEstimate(3)).rejects.toThrow('DELETE failed');
	});
});

describe('updateEstimateStatus', () => {
	it('is an async function', () => {
		expect(updateEstimateStatus(1, 'accepted')).toBeInstanceOf(Promise);
	});
});

describe('bulkDeleteEstimates', () => {
	it('does nothing for empty array', async () => {
		await bulkDeleteEstimates([]);
		// Should not throw
	});

	it('is an async function', () => {
		expect(bulkDeleteEstimates([1, 2, 3])).toBeInstanceOf(Promise);
	});
});

describe('bulkUpdateEstimateStatus', () => {
	it('does nothing for empty array', async () => {
		await bulkUpdateEstimateStatus([], 'sent');
		// Should not throw
	});

	it('is an async function', () => {
		expect(bulkUpdateEstimateStatus([1, 2], 'sent')).toBeInstanceOf(Promise);
	});
});

describe('getClientEstimates', () => {
	it('is an async function', () => {
		expect(getClientEstimates(5)).toBeInstanceOf(Promise);
	});
});

describe('convertEstimateToInvoice', () => {
	it('is an async function', () => {
		mockDb.then = (resolve: any) => resolve([]);
		const p = convertEstimateToInvoice(999);
		expect(p).toBeInstanceOf(Promise);
		p.catch(() => {}); // suppress unhandled rejection
	});

	it('throws when estimate not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		await expect(convertEstimateToInvoice(999)).rejects.toThrow('Estimate not found');
	});

	it('throws when estimate is not accepted', async () => {
		mockDb.then = (resolve: any) => resolve([{ id: 1, status: 'draft', converted_invoice_id: null }]);
		await expect(convertEstimateToInvoice(1)).rejects.toThrow('Only accepted estimates can be converted');
	});

	it('throws when estimate is already converted', async () => {
		mockDb.then = (resolve: any) => resolve([{ id: 1, status: 'accepted', converted_invoice_id: 5 }]);
		await expect(convertEstimateToInvoice(1)).rejects.toThrow('already been converted');
	});
});
