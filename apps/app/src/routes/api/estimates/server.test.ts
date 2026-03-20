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
		estimates: {
			getEstimates: vi.fn(),
			getEstimate: vi.fn(),
			createEstimate: vi.fn(),
			updateEstimate: vi.fn(),
			deleteEstimate: vi.fn(),
			bulkDeleteEstimates: vi.fn(),
			bulkUpdateEstimateStatus: vi.fn(),
			updateEstimateStatus: vi.fn(),
			duplicateEstimate: vi.fn(),
			convertEstimateToInvoice: vi.fn()
		}
	}
}));

vi.mock('$lib/validation/validate.js', () => ({
	validate: (_schema: unknown, data: unknown) => data
}));

vi.mock('$lib/validation/schemas.js', () => ({
	CreateEstimateSchema: {},
	BulkDeleteSchema: {},
	SearchParamsSchema: {}
}));

vi.mock('$lib/server/db-error.js', () => ({
	dbError: (err: unknown) => {
		throw err;
	},
	fkOrNull: (val: unknown) => {
		const n = Number(val);
		return Number.isFinite(n) && n > 0 ? n : null;
	}
}));

import { GET, POST } from './+server.js';
import { GET as GET_ID, PUT, DELETE, PATCH } from './[id]/+server.js';
import { repositories } from '$lib/repositories/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}

function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/estimates', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('calls getEstimates with search, status, and pagination', async () => {
		vi.mocked(repositories.estimates.getEstimates).mockResolvedValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/estimates', { search: 'proj', status: 'draft', page: '3', limit: '10' });
		const result = await GET({ url } as any);

		expect(repositories.estimates.getEstimates).toHaveBeenCalledWith('proj', 'draft', { page: 3, limit: 10 });
		expect((result as any).body).toEqual({ data: [], total: 0 });
	});

	it('uses default pagination', async () => {
		vi.mocked(repositories.estimates.getEstimates).mockResolvedValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/estimates');
		await GET({ url } as any);

		expect(repositories.estimates.getEstimates).toHaveBeenCalledWith(undefined, undefined, { page: 1, limit: 50 });
	});

	it('caps limit at 200', async () => {
		vi.mocked(repositories.estimates.getEstimates).mockResolvedValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/estimates', { limit: '999' });
		await GET({ url } as any);

		expect(repositories.estimates.getEstimates).toHaveBeenCalledWith(undefined, undefined, { page: 1, limit: 200 });
	});

	it('rejects search longer than 255 chars', async () => {
		const url = makeUrl('/api/estimates', { search: 'z'.repeat(256) });
		try {
			await GET({ url } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Search query too long');
		}
	});
});

describe('POST /api/estimates', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('creates an estimate with lineItems and returns 201', async () => {
		vi.mocked(repositories.estimates.createEstimate).mockResolvedValue(20 as any);

		const lineItems = [{ description: 'Service', quantity: 2, unit_price: 75 }];
		const request = makeRequest({ client_id: 3, estimate_number: 'EST-001', lineItems });
		const result = await POST({ request } as any);

		expect(repositories.estimates.createEstimate).toHaveBeenCalledWith(
			expect.objectContaining({ client_id: 3, estimate_number: 'EST-001' }),
			lineItems
		);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 20 });
	});

	it('defaults lineItems to empty array', async () => {
		vi.mocked(repositories.estimates.createEstimate).mockResolvedValue(1 as any);

		const request = makeRequest({ client_id: 1 });
		await POST({ request } as any);

		expect(repositories.estimates.createEstimate).toHaveBeenCalledWith(expect.anything(), []);
	});

	it('applies fkOrNull to client_id and payer_id', async () => {
		vi.mocked(repositories.estimates.createEstimate).mockResolvedValue(1 as any);

		const request = makeRequest({ client_id: 0, payer_id: '' });
		await POST({ request } as any);

		const calledWith = vi.mocked(repositories.estimates.createEstimate).mock.calls[0][0];
		expect(calledWith.client_id).toBeNull();
		expect((calledWith as any).payer_id).toBeNull();
	});

	it('bulk-deletes estimates', async () => {
		vi.mocked(repositories.estimates.bulkDeleteEstimates).mockResolvedValue(undefined as any);

		const request = makeRequest({ action: 'bulk-delete', ids: [1, 2] });
		const result = await POST({ request } as any);

		expect(repositories.estimates.bulkDeleteEstimates).toHaveBeenCalledWith([1, 2]);
		expect((result as any).body).toEqual({ success: true });
	});

	it('bulk-updates estimate status', async () => {
		vi.mocked(repositories.estimates.bulkUpdateEstimateStatus).mockResolvedValue(undefined as any);

		const request = makeRequest({ action: 'bulk-status', ids: [3, 4], status: 'accepted' });
		const result = await POST({ request } as any);

		expect(repositories.estimates.bulkUpdateEstimateStatus).toHaveBeenCalledWith([3, 4], 'accepted');
		expect((result as any).body).toEqual({ success: true });
	});
});

describe('GET /api/estimates/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('returns an estimate by id', async () => {
		const mockEstimate = { id: 1, estimate_number: 'EST-001' };
		vi.mocked(repositories.estimates.getEstimate).mockResolvedValue(mockEstimate as any);

		const result = await GET_ID({ params: { id: '1' } } as any);
		expect(repositories.estimates.getEstimate).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual(mockEstimate);
	});

	it('throws 404 if estimate not found', async () => {
		vi.mocked(repositories.estimates.getEstimate).mockResolvedValue(undefined as any);

		try {
			await GET_ID({ params: { id: '999' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(404);
			expect(e.body.message).toBe('Estimate not found');
		}
	});

	it('throws 400 for invalid id', async () => {
		try {
			await GET_ID({ params: { id: 'bad' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Invalid ID');
		}
	});
});

describe('PUT /api/estimates/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('updates an estimate with lineItems', async () => {
		vi.mocked(repositories.estimates.updateEstimate).mockResolvedValue(undefined as any);

		const lineItems = [{ description: 'Updated', quantity: 1, unit_price: 200 }];
		const request = makeRequest({ client_id: 5, payer_id: 2, lineItems });
		const result = await PUT({ params: { id: '1' }, request } as any);

		expect(repositories.estimates.updateEstimate).toHaveBeenCalledWith(
			1,
			expect.objectContaining({ client_id: 5, payer_id: 2 }),
			lineItems
		);
		expect((result as any).body).toEqual({ success: true });
	});

	it('defaults lineItems to empty array', async () => {
		vi.mocked(repositories.estimates.updateEstimate).mockResolvedValue(undefined as any);

		const request = makeRequest({ client_id: 1 });
		await PUT({ params: { id: '1' }, request } as any);

		expect(repositories.estimates.updateEstimate).toHaveBeenCalledWith(1, expect.anything(), []);
	});

	it('throws 400 for invalid id', async () => {
		const request = makeRequest({});
		try {
			await PUT({ params: { id: '-1' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('DELETE /api/estimates/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('deletes an estimate', async () => {
		vi.mocked(repositories.estimates.deleteEstimate).mockResolvedValue(undefined as any);

		const result = await DELETE({ params: { id: '7' } } as any);
		expect(repositories.estimates.deleteEstimate).toHaveBeenCalledWith(7);
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
});

describe('PATCH /api/estimates/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('updates estimate status', async () => {
		vi.mocked(repositories.estimates.updateEstimateStatus).mockResolvedValue(undefined as any);

		const request = makeRequest({ action: 'status', status: 'accepted' });
		const result = await PATCH({ params: { id: '1' }, request } as any);

		expect(repositories.estimates.updateEstimateStatus).toHaveBeenCalledWith(1, 'accepted');
		expect((result as any).body).toEqual({ success: true });
	});

	it('duplicates an estimate', async () => {
		const dupeResult = { id: 50, estimate_number: 'EST-002' };
		vi.mocked(repositories.estimates.duplicateEstimate).mockResolvedValue(dupeResult as any);

		const request = makeRequest({ action: 'duplicate' });
		const result = await PATCH({ params: { id: '1' }, request } as any);

		expect(repositories.estimates.duplicateEstimate).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual(dupeResult);
	});

	it('converts an estimate to an invoice', async () => {
		const convertResult = { invoice_id: 100 };
		vi.mocked(repositories.estimates.convertEstimateToInvoice).mockResolvedValue(convertResult as any);

		const request = makeRequest({ action: 'convert' });
		const result = await PATCH({ params: { id: '1' }, request } as any);

		expect(repositories.estimates.convertEstimateToInvoice).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual(convertResult);
	});

	it('throws 400 for unknown action', async () => {
		const request = makeRequest({ action: 'bogus' });
		try {
			await PATCH({ params: { id: '1' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Unknown action');
		}
	});

	it('throws 400 for invalid id', async () => {
		const request = makeRequest({ action: 'status', status: 'draft' });
		try {
			await PATCH({ params: { id: 'abc' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Invalid ID');
		}
	});
});
