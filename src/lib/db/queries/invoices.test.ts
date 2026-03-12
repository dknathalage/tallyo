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
	getClientInvoices,
	markOverdueInvoices,
	bulkDeleteInvoices,
	duplicateInvoice
} from './invoices.js';
import { query, execute, save, runRaw } from '../connection.svelte.js';
import { logAudit } from '../audit.js';

const mockLogAudit = vi.mocked(logAudit);

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
	it('deletes invoice and audit logs inside a transaction', async () => {
		await deleteInvoice(3);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM invoices WHERE id = ?', [3]);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('rolls back when the delete fails, preventing orphaned audit log entries', async () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('DELETE failed');
		});

		await expect(deleteInvoice(3)).rejects.toThrow('DELETE failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
	});

	it('rolls back when the audit log write fails, preventing silent data loss', async () => {
		// logAudit is the audit boundary — if it throws (e.g. DB constraint), the
		// transaction must roll back so the invoice delete is also undone.
		mockLogAudit.mockImplementationOnce(() => { throw new Error('audit write failed'); });

		await expect(deleteInvoice(3)).rejects.toThrow('audit write failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
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

describe('markOverdueInvoices', () => {
	it('does nothing and returns 0 when no sent invoices are overdue', async () => {
		mockQuery.mockReturnValue([]);

		const count = await markOverdueInvoices();

		expect(count).toBe(0);
		expect(mockExecute).not.toHaveBeenCalled();
		expect(mockSave).not.toHaveBeenCalled();
	});

	it('updates overdue invoices and returns count', async () => {
		// First query: find overdue sent invoices; subsequent calls for getInvoice return null
		mockQuery.mockReturnValueOnce([{ id: 1 }, { id: 2 }]).mockReturnValue([]);

		const count = await markOverdueInvoices();

		expect(count).toBe(2);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining("UPDATE invoices SET status = 'overdue'"),
			[1, 2]
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('only selects invoices with status sent and past due date', async () => {
		mockQuery.mockReturnValue([]);

		await markOverdueInvoices();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("status = 'sent' AND due_date < date('now')")
		);
	});

	it('logs an audit entry for each overdue invoice', async () => {
		mockQuery.mockReturnValueOnce([{ id: 3 }]).mockReturnValue([]);

		await markOverdueInvoices();

		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'invoice',
				entity_id: 3,
				action: 'status_change',
				changes: { status: { old: 'sent', new: 'overdue' } }
			})
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
		expect(mockSave).not.toHaveBeenCalled();
	});

	it('deletes all specified invoices within a transaction', async () => {
		mockQuery.mockReturnValue([]);

		await bulkDeleteInvoices([1, 2, 3]);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('DELETE FROM invoices WHERE id IN'),
			[1, 2, 3]
		);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('also deletes associated line_items for all specified invoices', async () => {
		mockQuery.mockReturnValue([]);

		await bulkDeleteInvoices([4, 5]);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('DELETE FROM line_items WHERE invoice_id IN'),
			[4, 5]
		);
	});

	it('audit logs a delete entry for each invoice id', async () => {
		// Return null for each getInvoice call (query returns [])
		mockQuery.mockReturnValue([]);

		await bulkDeleteInvoices([10, 11]);

		expect(mockLogAudit).toHaveBeenCalledTimes(2);
		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({ entity_type: 'invoice', entity_id: 10, action: 'delete' })
		);
		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({ entity_type: 'invoice', entity_id: 11, action: 'delete' })
		);
	});

	it('all audit entries share the same batch_id', async () => {
		mockQuery.mockReturnValue([]);

		await bulkDeleteInvoices([20, 21, 22]);

		const batchIds = mockLogAudit.mock.calls.map((c) => (c[0] as { batch_id: string }).batch_id);
		expect(new Set(batchIds).size).toBe(1);
		expect(typeof batchIds[0]).toBe('string');
	});

	it('rolls back on error', async () => {
		mockQuery.mockReturnValue([]);
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Bulk delete failed');
		});

		await expect(bulkDeleteInvoices([1, 2])).rejects.toThrow('Bulk delete failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
	});
});

vi.mock('../../utils/invoice-number.js', () => ({
	generateInvoiceNumber: vi.fn().mockReturnValue('INV-9999')
}));

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
			expect.arrayContaining(['INV-9999'])
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

	it('runs inside a transaction and saves', async () => {
		mockQuery
			.mockReturnValueOnce([originalInvoice])
			.mockReturnValueOnce(originalLineItems)
			.mockReturnValueOnce([{ id: 99 }]);

		await duplicateInvoice(1);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('audit logs the creation of the duplicate', async () => {
		mockQuery
			.mockReturnValueOnce([originalInvoice])
			.mockReturnValueOnce(originalLineItems)
			.mockReturnValueOnce([{ id: 99 }]);

		await duplicateInvoice(1);

		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'invoice',
				entity_id: 99,
				action: 'create'
			})
		);
	});

	it('throws when the original invoice does not exist', async () => {
		mockQuery.mockReturnValue([]);

		await expect(duplicateInvoice(999)).rejects.toThrow('Invoice 999 not found');
	});

	it('rolls back on error', async () => {
		mockQuery
			.mockReturnValueOnce([originalInvoice])
			.mockReturnValueOnce(originalLineItems);
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Insert failed');
		});

		await expect(duplicateInvoice(1)).rejects.toThrow('Insert failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});
});
