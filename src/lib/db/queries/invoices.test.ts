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
	generateInvoiceNumber: vi.fn().mockResolvedValue('INV-0100')
}));

vi.mock('./business-profile.js', () => ({
	getBusinessProfile: vi.fn().mockResolvedValue({ default_currency: 'USD' })
}));

import {
	getInvoices,
	getInvoice,
	getInvoiceLineItems,
	createInvoice,
	updateInvoice,
	deleteInvoice,
	updateInvoiceStatus,
	getClientInvoices,
	markOverdueInvoices,
	bulkDeleteInvoices,
	duplicateInvoice,
	getAgingReport
} from './invoices.js';

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

describe('getInvoices', () => {
	it('is an async function', () => {
		expect(getInvoices()).toBeInstanceOf(Promise);
	});

	it('returns paginated result', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getInvoices();
		expect(result).toHaveProperty('data');
		expect(result).toHaveProperty('total');
	});
});

describe('getInvoice', () => {
	it('is an async function', () => {
		expect(getInvoice(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getInvoice(999);
		expect(result).toBeNull();
	});
});

describe('getInvoiceLineItems', () => {
	it('is an async function', () => {
		expect(getInvoiceLineItems(1)).toBeInstanceOf(Promise);
	});
});

describe('createInvoice', () => {
	it('is an async function', () => {
		mockDb.returning.mockReturnValue(mockDb);
		mockDb.all.mockReturnValue([{ id: 7 }]);
		expect(createInvoice({
			invoice_number: 'INV-0001',
			client_id: 1,
			date: '2025-01-01',
			due_date: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110
		}, [])).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockReturnValue(mockDb);
		mockDb.all.mockReturnValue([{ id: 7 }]);
		const id = await createInvoice({
			invoice_number: 'INV-0001',
			client_id: 1,
			date: '2025-01-01',
			due_date: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110
		}, []);
		expect(id).toBe(7);
	});

	it('propagates errors', async () => {
		mockDb.transaction.mockRejectedValueOnce(new Error('SQL error'));
		await expect(createInvoice({
			invoice_number: 'INV-0001',
			client_id: 1,
			date: '2025-01-01',
			due_date: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110
		}, [])).rejects.toThrow('SQL error');
	});
});

describe('updateInvoice', () => {
	it('is an async function', () => {
		expect(updateInvoice(1, {
			invoice_number: 'INV-0001',
			client_id: 1,
			date: '2025-01-01',
			due_date: '2025-02-01',
			subtotal: 200,
			tax_rate: 10,
			tax_amount: 20,
			total: 220,
			notes: 'Updated',
			status: 'sent'
		}, [])).toBeInstanceOf(Promise);
	});
});

describe('deleteInvoice', () => {
	it('is an async function', () => {
		expect(deleteInvoice(3)).toBeInstanceOf(Promise);
	});

	it('propagates errors', async () => {
		mockDb.where.mockRejectedValueOnce(new Error('DELETE failed'));
		await expect(deleteInvoice(3)).rejects.toThrow('DELETE failed');
	});
});

describe('updateInvoiceStatus', () => {
	it('is an async function', () => {
		expect(updateInvoiceStatus(1, 'paid')).toBeInstanceOf(Promise);
	});
});

describe('getClientInvoices', () => {
	it('is an async function', () => {
		expect(getClientInvoices(5)).toBeInstanceOf(Promise);
	});
});

describe('markOverdueInvoices', () => {
	it('is an async function', () => {
		expect(markOverdueInvoices()).toBeInstanceOf(Promise);
	});

	it('returns empty array when no sent invoices are overdue', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await markOverdueInvoices();
		expect(result).toEqual([]);
	});
});

describe('getAgingReport', () => {
	it('is an async function', () => {
		expect(getAgingReport()).toBeInstanceOf(Promise);
	});

	it('returns 5 aging buckets', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getAgingReport();
		expect(result).toHaveLength(5);
		expect(result.map((b: any) => b.label)).toEqual([
			'Current',
			'1–30 days',
			'31–60 days',
			'61–90 days',
			'90+ days'
		]);
	});

	it('returns all empty buckets when no outstanding invoices', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getAgingReport();
		result.forEach((b: any) => {
			expect(b.total).toBe(0);
			expect(b.invoices).toHaveLength(0);
		});
	});
});

describe('bulkDeleteInvoices', () => {
	it('does nothing when given an empty array', async () => {
		await bulkDeleteInvoices([]);
		// Should not throw
	});

	it('is an async function', () => {
		expect(bulkDeleteInvoices([1, 2, 3])).toBeInstanceOf(Promise);
	});

	it('propagates errors', async () => {
		mockDb.where.mockRejectedValueOnce(new Error('Bulk delete failed'));
		await expect(bulkDeleteInvoices([1, 2])).rejects.toThrow('Bulk delete failed');
	});
});

describe('duplicateInvoice', () => {
	it('is an async function', () => {
		mockDb.then = (resolve: any) => resolve([]);
		const p = duplicateInvoice(999);
		expect(p).toBeInstanceOf(Promise);
		p.catch(() => {}); // suppress unhandled rejection
	});

	it('throws when the original invoice does not exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		await expect(duplicateInvoice(999)).rejects.toThrow('Invoice 999 not found');
	});
});
