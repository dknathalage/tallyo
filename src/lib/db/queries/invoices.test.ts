import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined),
	runRaw: vi.fn()
}));

vi.mock('../audit.js', () => ({
	logAudit: vi.fn(),
	computeChanges: vi.fn().mockReturnValue({})
}));

import {
	getInvoices,
	getInvoice,
	getInvoiceLineItems,
	createInvoice,
	updateInvoice,
	deleteInvoice,
	updateInvoiceStatus,
	getClientInvoices
} from './invoices.js';
import { query, execute, save, runRaw } from '../connection.svelte.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);
const mockRunRaw = vi.mocked(runRaw);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getInvoices', () => {
	it('returns all invoices with no filters', () => {
		mockQuery.mockReturnValue([]);
		getInvoices();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id ORDER BY i.created_at DESC'),
			[]
		);
	});

	it('filters by search term', () => {
		mockQuery.mockReturnValue([]);
		getInvoices('INV-001');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('i.invoice_number LIKE ? OR c.name LIKE ?'),
			['%INV-001%', '%INV-001%']
		);
	});

	it('filters by status', () => {
		mockQuery.mockReturnValue([]);
		getInvoices(undefined, 'paid');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('i.status = ?'),
			['paid']
		);
	});

	it('filters by both search and status', () => {
		mockQuery.mockReturnValue([]);
		getInvoices('test', 'draft');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE'),
			['%test%', '%test%', 'draft']
		);
	});
});

describe('getInvoice', () => {
	it('returns invoice when found', () => {
		const invoice = { id: 1, invoice_number: 'INV-0001' };
		mockQuery.mockReturnValue([invoice]);

		expect(getInvoice(1)).toEqual(invoice);
		expect(mockQuery).toHaveBeenCalledWith(expect.stringContaining('WHERE i.id = ?'), [1]);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		expect(getInvoice(999)).toBeNull();
	});
});

describe('getInvoiceLineItems', () => {
	it('returns line items for an invoice', () => {
		const items = [{ id: 1, description: 'Service', quantity: 1, rate: 100, amount: 100 }];
		mockQuery.mockReturnValue(items);

		expect(getInvoiceLineItems(1)).toEqual(items);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE invoice_id = ? ORDER BY sort_order'),
			[1]
		);
	});
});

describe('createInvoice', () => {
	const invoiceData = {
		invoice_number: 'INV-0001',
		client_id: 1,
		date: '2025-01-01',
		due_date: '2025-02-01',
		subtotal: 100,
		tax_rate: 10,
		tax_amount: 10,
		total: 110
	};

	const lineItems = [
		{ description: 'Service A', quantity: 1, rate: 100, amount: 100, sort_order: 0, notes: 'Test note' }
	];

	it('creates invoice with line items in a transaction', async () => {
		mockQuery.mockReturnValue([{ id: 7 }]);

		const id = await createInvoice(invoiceData, lineItems);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['INV-0001', 1])
		);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO line_items'),
			[expect.any(String), 7, 'Service A', 1, 100, 100, 'Test note', 0]
		);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
		expect(id).toBe(7);
	});

	it('rolls back on error', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);
		mockExecute.mockImplementationOnce(() => {
			throw new Error('SQL error');
		});

		await expect(createInvoice(invoiceData, lineItems)).rejects.toThrow('SQL error');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});

	it('defaults optional fields', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createInvoice(invoiceData, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['', 'draft'])
		);
	});

	it('includes snapshot fields when provided', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		const dataWithSnapshots = {
			...invoiceData,
			business_snapshot: '{"name":"My Biz"}',
			client_snapshot: '{"name":"Client A"}',
			payer_snapshot: '{"name":"Payer X"}'
		};

		await createInvoice(dataWithSnapshots, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('business_snapshot'),
			expect.arrayContaining(['{"name":"My Biz"}', '{"name":"Client A"}', '{"name":"Payer X"}'])
		);
	});

	it('defaults snapshot fields to empty JSON object', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createInvoice(invoiceData, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['{}', '{}', '{}'])
		);
	});
});

describe('updateInvoice', () => {
	const invoiceData = {
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
	};

	it('updates invoice and replaces line items in a transaction', async () => {
		const newItems = [
			{ description: 'New Service', quantity: 2, rate: 100, amount: 200, sort_order: 0, notes: 'Updated note' }
		];

		await updateInvoice(1, invoiceData, newItems);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE invoices SET'),
			expect.arrayContaining(['INV-0001', 1])
		);
		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM line_items WHERE invoice_id = ?', [1]);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO line_items'),
			[expect.any(String), 1, 'New Service', 2, 100, 200, 'Updated note', 0]
		);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('rolls back on error', async () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Update failed');
		});

		await expect(updateInvoice(1, invoiceData, [])).rejects.toThrow('Update failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});

	it('includes snapshot fields in update', async () => {
		const dataWithSnapshots = {
			...invoiceData,
			business_snapshot: '{"name":"Updated Biz"}',
			client_snapshot: '{"name":"Updated Client"}',
			payer_snapshot: '{"name":"Updated Payer"}'
		};

		await updateInvoice(1, dataWithSnapshots, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('business_snapshot'),
			expect.arrayContaining(['{"name":"Updated Biz"}', '{"name":"Updated Client"}', '{"name":"Updated Payer"}'])
		);
	});
});

describe('deleteInvoice', () => {
	it('deletes invoice and saves', async () => {
		await deleteInvoice(3);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM invoices WHERE id = ?', [3]);
		expect(mockSave).toHaveBeenCalled();
	});
});

describe('updateInvoiceStatus', () => {
	it('updates status and saves', async () => {
		await updateInvoiceStatus(1, 'paid');

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE invoices SET status = ?'),
			['paid', 1]
		);
		expect(mockSave).toHaveBeenCalled();
	});
});

describe('getClientInvoices', () => {
	it('returns invoices for a specific client', () => {
		mockQuery.mockReturnValue([]);
		getClientInvoices(5);

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE i.client_id = ?'),
			[5]
		);
	});
});
