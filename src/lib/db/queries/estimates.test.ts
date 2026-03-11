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

vi.mock('../../utils/invoice-number.js', () => ({
	generateInvoiceNumber: vi.fn().mockReturnValue('INV-0001')
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
import { query, execute, save, runRaw } from '../connection.svelte.js';
import { logAudit } from '../audit.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);
const mockRunRaw = vi.mocked(runRaw);
const mockLogAudit = vi.mocked(logAudit);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getEstimates', () => {
	it('returns all estimates with no filters', () => {
		mockQuery.mockReturnValue([]);
		getEstimates();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id ORDER BY e.created_at DESC'),
			[]
		);
	});

	it('filters by search term', () => {
		mockQuery.mockReturnValue([]);
		getEstimates('EST-001');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('e.estimate_number LIKE ? OR c.name LIKE ?'),
			['%EST-001%', '%EST-001%']
		);
	});

	it('filters by status', () => {
		mockQuery.mockReturnValue([]);
		getEstimates(undefined, 'accepted');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('e.status = ?'),
			['accepted']
		);
	});

	it('filters by both search and status', () => {
		mockQuery.mockReturnValue([]);
		getEstimates('test', 'draft');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE'),
			['%test%', '%test%', 'draft']
		);
	});
});

describe('getEstimate', () => {
	it('returns estimate when found', () => {
		const estimate = { id: 1, estimate_number: 'EST-0001' };
		mockQuery.mockReturnValue([estimate]);

		expect(getEstimate(1)).toEqual(estimate);
		expect(mockQuery).toHaveBeenCalledWith(expect.stringContaining('WHERE e.id = ?'), [1]);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		expect(getEstimate(999)).toBeNull();
	});
});

describe('getEstimateLineItems', () => {
	it('returns line items for an estimate', () => {
		const items = [{ id: 1, description: 'Service', quantity: 1, rate: 100, amount: 100 }];
		mockQuery.mockReturnValue(items);

		expect(getEstimateLineItems(1)).toEqual(items);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE estimate_id = ? ORDER BY sort_order'),
			[1]
		);
	});
});

describe('createEstimate', () => {
	const estimateData = {
		estimate_number: 'EST-0001',
		client_id: 1,
		date: '2025-01-01',
		valid_until: '2025-02-01',
		subtotal: 100,
		tax_rate: 10,
		tax_amount: 10,
		total: 110
	};

	const lineItems = [
		{ description: 'Service A', quantity: 1, rate: 100, amount: 100, sort_order: 0, notes: 'Test note' }
	];

	it('creates estimate with line items in a transaction', async () => {
		mockQuery.mockReturnValue([{ id: 7 }]);

		const id = await createEstimate(estimateData, lineItems);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO estimates'),
			expect.arrayContaining(['EST-0001', 1])
		);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO estimate_line_items'),
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

		await expect(createEstimate(estimateData, lineItems)).rejects.toThrow('SQL error');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});

	it('defaults optional fields', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createEstimate(estimateData, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO estimates'),
			expect.arrayContaining(['', 'draft'])
		);
	});

	it('includes snapshot fields when provided', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		const dataWithSnapshots = {
			...estimateData,
			business_snapshot: '{"name":"My Biz"}',
			client_snapshot: '{"name":"Client A"}',
			payer_snapshot: '{"name":"Payer X"}'
		};

		await createEstimate(dataWithSnapshots, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('business_snapshot'),
			expect.arrayContaining(['{"name":"My Biz"}', '{"name":"Client A"}', '{"name":"Payer X"}'])
		);
	});

	it('defaults snapshot fields to empty JSON object', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createEstimate(estimateData, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO estimates'),
			expect.arrayContaining(['{}', '{}', '{}'])
		);
	});
});

describe('updateEstimate', () => {
	const estimateData = {
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
	};

	it('updates estimate and replaces line items in a transaction', async () => {
		const newItems = [
			{ description: 'New Service', quantity: 2, rate: 100, amount: 200, sort_order: 0, notes: 'Updated note' }
		];

		await updateEstimate(1, estimateData, newItems);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE estimates SET'),
			expect.arrayContaining(['EST-0001', 1])
		);
		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM estimate_line_items WHERE estimate_id = ?', [1]);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO estimate_line_items'),
			[expect.any(String), 1, 'New Service', 2, 100, 200, 'Updated note', 0]
		);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('rolls back on error', async () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('Update failed');
		});

		await expect(updateEstimate(1, estimateData, [])).rejects.toThrow('Update failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});

	it('includes snapshot fields in update', async () => {
		const dataWithSnapshots = {
			...estimateData,
			business_snapshot: '{"name":"Updated Biz"}',
			client_snapshot: '{"name":"Updated Client"}',
			payer_snapshot: '{"name":"Updated Payer"}'
		};

		await updateEstimate(1, dataWithSnapshots, []);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('business_snapshot'),
			expect.arrayContaining(['{"name":"Updated Biz"}', '{"name":"Updated Client"}', '{"name":"Updated Payer"}'])
		);
	});
});

describe('deleteEstimate', () => {
	it('deletes estimate and audit logs inside a transaction', async () => {
		await deleteEstimate(3);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM estimates WHERE id = ?', [3]);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('rolls back when the delete fails', async () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('DELETE failed');
		});

		await expect(deleteEstimate(3)).rejects.toThrow('DELETE failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
	});

	it('rolls back when the audit log write fails', async () => {
		mockLogAudit.mockImplementationOnce(() => { throw new Error('audit write failed'); });

		await expect(deleteEstimate(3)).rejects.toThrow('audit write failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
	});
});

describe('updateEstimateStatus', () => {
	it('updates status and saves', async () => {
		await updateEstimateStatus(1, 'accepted');

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE estimates SET status = ?'),
			['accepted', 1]
		);
		expect(mockSave).toHaveBeenCalled();
	});
});

describe('bulkDeleteEstimates', () => {
	it('does nothing for empty array', async () => {
		await bulkDeleteEstimates([]);
		expect(mockRunRaw).not.toHaveBeenCalled();
	});

	it('deletes multiple estimates in a transaction', async () => {
		mockQuery.mockReturnValue([]);
		await bulkDeleteEstimates([1, 2, 3]);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('DELETE FROM estimate_line_items WHERE estimate_id IN'),
			[1, 2, 3]
		);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('DELETE FROM estimates WHERE id IN'),
			[1, 2, 3]
		);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});
});

describe('bulkUpdateEstimateStatus', () => {
	it('does nothing for empty array', async () => {
		await bulkUpdateEstimateStatus([], 'sent');
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('updates status for multiple estimates', async () => {
		mockQuery.mockReturnValue([]);
		await bulkUpdateEstimateStatus([1, 2], 'sent');

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE estimates SET status = ?'),
			['sent', 1, 2]
		);
		expect(mockSave).toHaveBeenCalled();
	});
});

describe('getClientEstimates', () => {
	it('returns estimates for a specific client', () => {
		mockQuery.mockReturnValue([]);
		getClientEstimates(5);

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE e.client_id = ?'),
			[5]
		);
	});
});

describe('convertEstimateToInvoice', () => {
	it('throws when estimate not found', async () => {
		mockQuery.mockReturnValue([]);

		await expect(convertEstimateToInvoice(999)).rejects.toThrow('Estimate not found');
	});

	it('throws when estimate is not accepted', async () => {
		mockQuery.mockReturnValueOnce([{ id: 1, status: 'draft', converted_invoice_id: null }]);

		await expect(convertEstimateToInvoice(1)).rejects.toThrow('Only accepted estimates can be converted');
	});

	it('throws when estimate is already converted', async () => {
		mockQuery.mockReturnValueOnce([{ id: 1, status: 'accepted', converted_invoice_id: 5 }]);

		await expect(convertEstimateToInvoice(1)).rejects.toThrow('already been converted');
	});

	it('converts accepted estimate to invoice', async () => {
		const estimate = {
			id: 1,
			status: 'accepted',
			converted_invoice_id: null,
			estimate_number: 'EST-0001',
			client_id: 2,
			date: '2025-01-01',
			valid_until: '2025-02-01',
			subtotal: 100,
			tax_rate: 10,
			tax_amount: 10,
			total: 110,
			notes: 'Test',
			currency_code: 'USD',
			business_snapshot: '{}',
			client_snapshot: '{}',
			payer_snapshot: '{}'
		};

		// First call: getEstimate in convertEstimateToInvoice
		mockQuery.mockReturnValueOnce([estimate]);
		// Second call: getEstimateLineItems
		mockQuery.mockReturnValueOnce([{ id: 1, description: 'Service', quantity: 1, rate: 100, amount: 100, notes: '', sort_order: 0 }]);
		// Third call: generateInvoiceNumber (via mock)
		// Fourth call: last_insert_rowid for invoice
		mockQuery.mockReturnValueOnce([{ id: 42 }]);

		const invoiceId = await convertEstimateToInvoice(1);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO invoices'),
			expect.arrayContaining(['INV-0001', 2])
		);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO line_items'),
			expect.arrayContaining([42, 'Service'])
		);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE estimates SET converted_invoice_id'),
			[42, 1]
		);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
		expect(invoiceId).toBe(42);
	});
});
