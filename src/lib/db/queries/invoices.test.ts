import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn()
}));

vi.mock('../number-generators.js', () => ({
	generateInvoiceNumber: vi.fn().mockReturnValue('INV-0100')
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
	duplicateInvoice
} from './invoices.js';
import { query, execute } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);

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

	it('creates invoice with line items and returns id', async () => {
		mockQuery.mockReturnValue([{ id: 7 }]);

		const id = await createInvoice(invoiceData, lineItems);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['INV-0001', 1])
		);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO line_items'),
			[expect.any(String), 7, 'Service A', 1, 100, 100, 'Test note', 0]
		);
		// Transaction management, audit, and save() are now the repository's responsibility
		expect(id).toBe(7);
	});

	it('propagates execute errors', () => {
		mockQuery.mockReturnValue([{ id: 1 }]);
		mockExecute.mockImplementationOnce(() => {
			throw new Error('SQL error');
		});

		expect(() => createInvoice(invoiceData, lineItems)).toThrow('SQL error');
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

	it('updates invoice and replaces line items', async () => {
		mockQuery.mockReturnValue([]); // no tax_rate_id lookup needed
		const newItems = [
			{ description: 'New Service', quantity: 2, rate: 100, amount: 200, sort_order: 0, notes: 'Updated note' }
		];

		await updateInvoice(1, invoiceData, newItems);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE invoices SET'),
			expect.arrayContaining(['INV-0001', 1])
		);
		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM line_items WHERE invoice_id = ?', [1]);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO line_items'),
			[expect.any(String), 1, 'New Service', 2, 100, 200, 'Updated note', 0]
		);
		// Transaction management, audit, and save() are now the repository's responsibility
	});

	it('propagates execute errors', () => {
		mockQuery.mockReturnValue([]); // no tax_rate_id lookup
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Update failed');
		});

		expect(() => updateInvoice(1, invoiceData, [])).toThrow('Update failed');
	});

	it('includes snapshot fields in update', async () => {
		mockQuery.mockReturnValue([]);
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
	it('deletes invoice', async () => {
		await deleteInvoice(3);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM invoices WHERE id = ?', [3]);
		// Transaction management, logAudit(), and save() are now the repository's responsibility
	});

	it('propagates execute errors', () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('DELETE failed');
		});

		expect(() => deleteInvoice(3)).toThrow('DELETE failed');
	});
});

describe('updateInvoiceStatus', () => {
	it('updates status', async () => {
		await updateInvoiceStatus(1, 'paid');

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE invoices SET status = ?'),
			['paid', 1]
		);
		// save() and logAudit() are now the repository's responsibility
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

describe('markOverdueInvoices', () => {
	it('returns empty array and does nothing when no sent invoices are overdue', async () => {
		mockQuery.mockReturnValue([]);

		const result = await markOverdueInvoices();

		expect(result).toEqual([]);
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('updates overdue invoices and returns their id+invoice_number pairs', async () => {
		mockQuery.mockReturnValueOnce([
			{ id: 1, invoice_number: 'INV-0001' },
			{ id: 2, invoice_number: 'INV-0002' }
		]);

		const result = await markOverdueInvoices();

		expect(result).toEqual([
			{ id: 1, invoice_number: 'INV-0001' },
			{ id: 2, invoice_number: 'INV-0002' }
		]);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining("UPDATE invoices SET status = 'overdue'"),
			[1, 2]
		);
		// logAudit() and save() are now the repository's responsibility
	});

	it('only selects invoices with status sent and past due date', async () => {
		mockQuery.mockReturnValue([]);

		await markOverdueInvoices();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("status = 'sent' AND due_date < date('now')")
		);
	});
});

import { getAgingReport } from './invoices.js';

vi.mock('./business-profile.js', () => ({
	getBusinessProfile: vi.fn().mockReturnValue({ default_currency: 'USD' })
}));

describe('getAgingReport', () => {
	it('returns 5 aging buckets', () => {
		mockQuery.mockReturnValueOnce([]);
		const result = getAgingReport();
		expect(result).toHaveLength(5);
		expect(result.map((b) => b.label)).toEqual([
			'Current',
			'1–30 days',
			'31–60 days',
			'61–90 days',
			'90+ days'
		]);
	});

	it('distributes invoices to correct buckets by days overdue', () => {
		mockQuery.mockReturnValueOnce([
			{ id: 1, invoice_number: 'INV-001', total: 1000, currency_code: 'USD', days_overdue: -2, status: 'sent' },
			{ id: 2, invoice_number: 'INV-002', total: 500, currency_code: 'USD', days_overdue: 15, status: 'overdue' },
			{ id: 3, invoice_number: 'INV-003', total: 750, currency_code: 'USD', days_overdue: 45, status: 'overdue' },
			{ id: 4, invoice_number: 'INV-004', total: 200, currency_code: 'USD', days_overdue: 75, status: 'overdue' },
			{ id: 5, invoice_number: 'INV-005', total: 300, currency_code: 'USD', days_overdue: 120, status: 'overdue' }
		]);
		const result = getAgingReport();
		expect(result[0].invoices).toHaveLength(1); // Current
		expect(result[1].invoices).toHaveLength(1); // 1–30
		expect(result[2].invoices).toHaveLength(1); // 31–60
		expect(result[3].invoices).toHaveLength(1); // 61–90
		expect(result[4].invoices).toHaveLength(1); // 90+
		expect(result[0].total).toBe(1000);
		expect(result[4].total).toBe(300);
	});

	it('returns all empty buckets when no outstanding invoices', () => {
		mockQuery.mockReturnValueOnce([]);
		const result = getAgingReport();
		result.forEach((b) => {
			expect(b.total).toBe(0);
			expect(b.invoices).toHaveLength(0);
		});
	});
});

describe('bulkDeleteInvoices', () => {
	it('does nothing when given an empty array', async () => {
		await bulkDeleteInvoices([]);

		expect(mockExecute).not.toHaveBeenCalled();
		// save() and logAudit() are now the repository's responsibility
	});

	it('deletes all specified invoices', async () => {
		await bulkDeleteInvoices([1, 2, 3]);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('DELETE FROM invoices WHERE id IN'),
			[1, 2, 3]
		);
		// Transaction management, audit, and save() are now the repository's responsibility
	});

	it('also deletes associated line_items for all specified invoices', async () => {
		await bulkDeleteInvoices([4, 5]);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('DELETE FROM line_items WHERE invoice_id IN'),
			[4, 5]
		);
	});

	it('propagates execute errors', () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Bulk delete failed');
		});

		expect(() => bulkDeleteInvoices([1, 2])).toThrow('Bulk delete failed');
	});
});

describe('duplicateInvoice', () => {
	const originalInvoice = {
		id: 1,
		invoice_number: 'INV-0001',
		client_id: 5,
		date: '2025-01-01',
		due_date: '2025-02-01',
		subtotal: 100,
		tax_rate: 10,
		tax_amount: 10,
		total: 110,
		notes: 'Original note',
		status: 'sent',
		currency_code: 'USD',
		business_snapshot: '{"name":"Biz"}',
		client_snapshot: '{"name":"Client"}',
		payer_snapshot: '{}'
	};

	const originalLineItems = [
		{ id: 10, description: 'Service A', quantity: 1, rate: 100, amount: 100, notes: 'note', sort_order: 0 }
	];

	it('creates a new draft invoice with a new invoice number', async () => {
		// getInvoice returns original, getInvoiceLineItems returns line items, last_insert_rowid returns new id
		mockQuery
			.mockReturnValueOnce([originalInvoice]) // getInvoice
			.mockReturnValueOnce(originalLineItems)  // getInvoiceLineItems
			.mockReturnValueOnce([{ id: 99 }]);      // last_insert_rowid

		const newId = await duplicateInvoice(1);

		expect(newId).toBe(99);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['INV-0100'])
		);
	});

	it('new invoice is created with status draft', async () => {
		mockQuery
			.mockReturnValueOnce([originalInvoice])
			.mockReturnValueOnce(originalLineItems)
			.mockReturnValueOnce([{ id: 99 }]);

		await duplicateInvoice(1);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['draft'])
		);
	});

	it('copies original line items to the new invoice', async () => {
		mockQuery
			.mockReturnValueOnce([originalInvoice])
			.mockReturnValueOnce(originalLineItems)
			.mockReturnValueOnce([{ id: 99 }]);

		await duplicateInvoice(1);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO line_items'),
			expect.arrayContaining([99, 'Service A', 1, 100, 100])
		);
	});

	it('throws when the original invoice does not exist', () => {
		mockQuery.mockReturnValue([]);

		expect(() => duplicateInvoice(999)).toThrow('Invoice 999 not found');
	});

	it('propagates execute errors', () => {
		mockQuery
			.mockReturnValueOnce([originalInvoice])
			.mockReturnValueOnce(originalLineItems);
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Insert failed');
		});

		expect(() => duplicateInvoice(1)).toThrow('Insert failed');
		// Transaction management, audit, and save() are now the repository's responsibility
	});
});
