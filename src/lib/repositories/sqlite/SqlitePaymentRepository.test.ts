import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/payments.js', () => ({
	getInvoicePayments: vi.fn(),
	getInvoiceTotalPaid: vi.fn(),
	createPayment: vi.fn(),
	deletePayment: vi.fn()
}));

import { SqlitePaymentRepository } from './SqlitePaymentRepository.js';
import * as queries from '$lib/db/queries/payments.js';

const mockGetInvoicePayments = vi.mocked(queries.getInvoicePayments);
const mockGetInvoiceTotalPaid = vi.mocked(queries.getInvoiceTotalPaid);
const mockCreatePayment = vi.mocked(queries.createPayment);
const mockDeletePayment = vi.mocked(queries.deletePayment);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqlitePaymentRepository', () => {
	describe('getInvoicePayments', () => {
		it('delegates to getInvoicePayments query', () => {
			const repo = new SqlitePaymentRepository();
			const payments = [{ id: 1, amount: 100 }] as any;
			mockGetInvoicePayments.mockReturnValue(payments);

			const result = repo.getInvoicePayments(5);
			expect(mockGetInvoicePayments).toHaveBeenCalledWith(5);
			expect(result).toBe(payments);
		});
	});

	describe('getInvoiceTotalPaid', () => {
		it('delegates to getInvoiceTotalPaid query', () => {
			const repo = new SqlitePaymentRepository();
			mockGetInvoiceTotalPaid.mockReturnValue(500);

			const result = repo.getInvoiceTotalPaid(3);
			expect(mockGetInvoiceTotalPaid).toHaveBeenCalledWith(3);
			expect(result).toBe(500);
		});

		it('returns 0 when no payments', () => {
			const repo = new SqlitePaymentRepository();
			mockGetInvoiceTotalPaid.mockReturnValue(0);

			expect(repo.getInvoiceTotalPaid(999)).toBe(0);
		});
	});

	describe('createPayment', () => {
		it('delegates to createPayment query', async () => {
			const repo = new SqlitePaymentRepository();
			mockCreatePayment.mockResolvedValue(7);

			const data = { invoice_id: 1, amount: 100, payment_date: '2025-01-15' };
			const id = await repo.createPayment(data);

			expect(mockCreatePayment).toHaveBeenCalledWith(data);
			expect(id).toBe(7);
		});

		it('delegates optional fields', async () => {
			const repo = new SqlitePaymentRepository();
			mockCreatePayment.mockResolvedValue(8);

			const data = { invoice_id: 2, amount: 200, payment_date: '2025-02-01', method: 'bank', notes: 'Full' };
			await repo.createPayment(data);

			expect(mockCreatePayment).toHaveBeenCalledWith(data);
		});
	});

	describe('deletePayment', () => {
		it('delegates to deletePayment query', async () => {
			const repo = new SqlitePaymentRepository();
			mockDeletePayment.mockResolvedValue(undefined);

			await repo.deletePayment(3);
			expect(mockDeletePayment).toHaveBeenCalledWith(3);
		});

		it('propagates errors from deletePayment', async () => {
			const repo = new SqlitePaymentRepository();
			mockDeletePayment.mockRejectedValue(new Error('delete failed'));

			await expect(repo.deletePayment(99)).rejects.toThrow('delete failed');
		});
	});
});
