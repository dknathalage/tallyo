import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

import {
	getInvoicePayments,
	getInvoiceTotalPaid,
	createPayment,
	deletePayment
} from './payments.js';
import { query, execute, save } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getInvoicePayments', () => {
	it('returns payments for an invoice ordered by date descending', () => {
		const payments = [
			{ id: 2, invoice_id: 1, amount: 50, payment_date: '2025-02-01', method: 'bank', notes: '' },
			{ id: 1, invoice_id: 1, amount: 100, payment_date: '2025-01-01', method: 'cash', notes: '' }
		];
		mockQuery.mockReturnValue(payments);

		const result = getInvoicePayments(1);

		expect(result).toEqual(payments);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE invoice_id = ?'),
			[1]
		);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('ORDER BY payment_date DESC'),
			[1]
		);
	});

	it('returns empty array when no payments exist', () => {
		mockQuery.mockReturnValue([]);

		const result = getInvoicePayments(99);

		expect(result).toEqual([]);
		expect(mockQuery).toHaveBeenCalledWith(expect.stringContaining('WHERE invoice_id = ?'), [99]);
	});
});

describe('getInvoiceTotalPaid', () => {
	it('returns the sum of payment amounts', () => {
		mockQuery.mockReturnValue([{ total: 150 }]);

		const result = getInvoiceTotalPaid(1);

		expect(result).toBe(150);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('SUM(amount)'),
			[1]
		);
	});

	it('returns 0 when no payments exist (SUM returns null)', () => {
		mockQuery.mockReturnValue([{ total: null }]);

		const result = getInvoiceTotalPaid(1);

		expect(result).toBe(0);
	});

	it('returns 0 when query returns empty array', () => {
		mockQuery.mockReturnValue([]);

		const result = getInvoiceTotalPaid(1);

		expect(result).toBe(0);
	});

	it('sums multiple partial payments correctly', () => {
		// The DB SUM handles this — we verify the function uses SUM and passes correct invoice id
		mockQuery.mockReturnValue([{ total: 275.5 }]);

		const result = getInvoiceTotalPaid(7);

		expect(result).toBe(275.5);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE invoice_id = ?'),
			[7]
		);
	});
});

describe('createPayment', () => {
	it('inserts a payment and returns its id', async () => {
		mockQuery.mockReturnValue([{ id: 5 }]);

		const id = await createPayment({
			invoice_id: 1,
			amount: 100,
			payment_date: '2025-01-15',
			method: 'bank',
			notes: 'Partial payment'
		});

		expect(id).toBe(5);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO payments'),
			expect.arrayContaining([1, 100, '2025-01-15', 'bank', 'Partial payment'])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('defaults method and notes to empty string when not provided', async () => {
		mockQuery.mockReturnValue([{ id: 3 }]);

		await createPayment({
			invoice_id: 2,
			amount: 50,
			payment_date: '2025-03-01'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO payments'),
			expect.arrayContaining([2, 50, '2025-03-01', '', ''])
		);
	});

	it('inserts a uuid as the first parameter', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createPayment({ invoice_id: 1, amount: 10, payment_date: '2025-01-01' });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(typeof args[0]).toBe('string');
		expect(args[0]).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
		);
	});
});

describe('deletePayment', () => {
	it('deletes a payment by id and saves', async () => {
		await deletePayment(4);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM payments WHERE id = ?', [4]);
		expect(mockSave).toHaveBeenCalled();
	});

	it('still calls execute and save even for non-existent payment id', async () => {
		// DB will silently do nothing; function should still complete and save
		await deletePayment(9999);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM payments WHERE id = ?', [9999]);
		expect(mockSave).toHaveBeenCalled();
	});
});
