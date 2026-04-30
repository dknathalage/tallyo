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
		taxRates: {
			getTaxRates: vi.fn(),
			createTaxRate: vi.fn(),
			updateTaxRate: vi.fn(),
			deleteTaxRate: vi.fn()
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

import { GET, POST } from './+server.js';
import { PUT, DELETE } from './[id]/+server.js';
import { repositories } from '$lib/repositories/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}

describe('GET /api/tax-rates', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns list of tax rates', async () => {
		const rates = [{ id: 1, name: 'VAT', rate: 20 }];
		vi.mocked(repositories.taxRates.getTaxRates).mockResolvedValue(rates as any);
		const result = await GET({} as any);
		expect((result as any).body).toEqual(rates);
	});
});

describe('POST /api/tax-rates', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a tax rate', async () => {
		vi.mocked(repositories.taxRates.createTaxRate).mockResolvedValue(7 as any);
		const result = await POST({ request: makeRequest({ name: 'GST', rate: 5 }) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 7 });
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.taxRates.createTaxRate).mockRejectedValue(new Error('DB fail'));
		await expect(POST({ request: makeRequest({ name: 'Bad' }) } as any)).rejects.toThrow('DB fail');
	});
});

describe('PUT /api/tax-rates/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('updates a tax rate', async () => {
		vi.mocked(repositories.taxRates.updateTaxRate).mockResolvedValue(undefined as any);
		const result = await PUT({ params: { id: '1' }, request: makeRequest({ rate: 15 }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.taxRates.updateTaxRate).toHaveBeenCalledWith(1, { rate: 15 });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await PUT({ params: { id: 'abc' }, request: makeRequest({}) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.taxRates.updateTaxRate).mockRejectedValue(new Error('DB error'));
		await expect(PUT({ params: { id: '1' }, request: makeRequest({}) } as any)).rejects.toThrow('DB error');
	});
});

describe('DELETE /api/tax-rates/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes a tax rate', async () => {
		vi.mocked(repositories.taxRates.deleteTaxRate).mockResolvedValue(undefined as any);
		const result = await DELETE({ params: { id: '1' } } as any);
		expect((result as any).body).toEqual({ success: true });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await DELETE({ params: { id: '0' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.taxRates.deleteTaxRate).mockRejectedValue(new Error('FK constraint'));
		await expect(DELETE({ params: { id: '1' } } as any)).rejects.toThrow('FK constraint');
	});
});
