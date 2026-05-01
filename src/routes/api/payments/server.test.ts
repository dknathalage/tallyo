import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('@sveltejs/kit', () => ({
	json: (data: unknown, opts?: { status?: number }) => {
		const status = opts?.status ?? 200;
		return { status, body: data, json: async () => data };
	},
	error: (status: number, message: string) => {
		const err = new Error(message);
		(err as any).status = status;
		(err as any).body = { message };
		throw err;
	}
}));

vi.mock('$lib/repositories/index.js', () => ({
	repositories: {
		payments: {
			getInvoicePayments: vi.fn(),
			createPayment: vi.fn(),
			deletePayment: vi.fn()
		}
	}
}));

vi.mock('$lib/server/db-error.js', () => ({
	dbError: (err: unknown) => { throw err; },
	fkOrNull: (val: unknown) => {
		const n = Number(val);
		return Number.isFinite(n) && n > 0 ? n : null;
	}
}));

vi.mock('$lib/validation/validate.js', () => ({
	validate: (_schema: unknown, data: unknown) => data
}));

vi.mock('$lib/validation/schemas.js', () => ({
	CreatePaymentSchema: {}
}));

import { GET, POST } from './+server.js';
import { DELETE } from './[id]/+server.js';
import { repositories } from '$lib/repositories/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}
function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/payments', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns empty array without invoiceId', async () => {
		const result = await GET({ url: makeUrl('/api/payments') } as any);
		expect((result as any).body).toEqual([]);
		expect(repositories.payments.getInvoicePayments).not.toHaveBeenCalled();
	});

	it('returns payments for invoiceId', async () => {
		const payments = [{ id: 1, amount: 100 }];
		vi.mocked(repositories.payments.getInvoicePayments).mockResolvedValue(payments as any);
		const result = await GET({ url: makeUrl('/api/payments', { invoiceId: '5' }) } as any);
		expect((result as any).body).toEqual(payments);
		expect(repositories.payments.getInvoicePayments).toHaveBeenCalledWith(5);
	});
});

describe('POST /api/payments', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a payment', async () => {
		vi.mocked(repositories.payments.createPayment).mockResolvedValue(10);
		const result = await POST({ request: makeRequest({ invoice_id: 5, amount: 100 }) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 10 });
		expect(repositories.payments.createPayment).toHaveBeenCalledWith({ invoice_id: 5, amount: 100 });
	});

	it('converts invalid invoice_id via fkOrNull', async () => {
		vi.mocked(repositories.payments.createPayment).mockResolvedValue(11);
		await POST({ request: makeRequest({ invoice_id: 'invalid', amount: 50 }) } as any);
		expect(repositories.payments.createPayment).toHaveBeenCalledWith({ invoice_id: null, amount: 50 });
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.payments.createPayment).mockRejectedValue(new Error('DB fail'));
		await expect(POST({ request: makeRequest({ invoice_id: 1, amount: 50 }) } as any)).rejects.toThrow('DB fail');
	});
});

describe('DELETE /api/payments/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes a payment', async () => {
		vi.mocked(repositories.payments.deletePayment).mockResolvedValue(undefined);
		const result = await DELETE({ params: { id: '3' } } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.payments.deletePayment).toHaveBeenCalledWith(3);
	});

	it('throws 400 for invalid id', async () => {
		try {
			await DELETE({ params: { id: 'abc' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('throws 400 for non-positive id', async () => {
		try {
			await DELETE({ params: { id: '0' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.payments.deletePayment).mockRejectedValue(new Error('FK error'));
		await expect(DELETE({ params: { id: '1' } } as any)).rejects.toThrow('FK error');
	});
});
