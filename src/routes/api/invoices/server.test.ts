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

vi.mock('$lib/repositories/sqlite/index.js', () => ({
	repositories: {
		invoices: {
			getInvoices: vi.fn(),
			getInvoice: vi.fn(),
			createInvoice: vi.fn(),
			updateInvoice: vi.fn(),
			deleteInvoice: vi.fn(),
			markOverdueInvoices: vi.fn(),
			updateInvoiceStatus: vi.fn(),
			duplicateInvoice: vi.fn()
		}
	}
}));

vi.mock('$lib/validation/validate.js', () => ({
	validate: (_schema: unknown, data: unknown) => data
}));

vi.mock('$lib/validation/schemas.js', () => ({
	CreateInvoiceSchema: {},
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
import { repositories } from '$lib/repositories/sqlite/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}

function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/invoices', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('calls markOverdueInvoices then getInvoices', () => {
		vi.mocked(repositories.invoices.getInvoices).mockReturnValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/invoices', { search: 'test', status: 'draft', page: '1', limit: '20' });
		GET({ url } as any);

		expect(repositories.invoices.markOverdueInvoices).toHaveBeenCalled();
		expect(repositories.invoices.getInvoices).toHaveBeenCalledWith('test', 'draft', { page: 1, limit: 20 });
	});

	it('uses default pagination', () => {
		vi.mocked(repositories.invoices.getInvoices).mockReturnValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/invoices');
		GET({ url } as any);

		expect(repositories.invoices.getInvoices).toHaveBeenCalledWith(undefined, undefined, { page: 1, limit: 50 });
	});

	it('caps limit at 200', () => {
		vi.mocked(repositories.invoices.getInvoices).mockReturnValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/invoices', { limit: '500' });
		GET({ url } as any);

		expect(repositories.invoices.getInvoices).toHaveBeenCalledWith(undefined, undefined, { page: 1, limit: 200 });
	});

	it('rejects search longer than 255 chars', () => {
		const url = makeUrl('/api/invoices', { search: 'x'.repeat(256) });
		try {
			GET({ url } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Search query too long');
		}
	});
});

describe('POST /api/invoices', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('creates an invoice with lineItems and returns 201', async () => {
		vi.mocked(repositories.invoices.createInvoice).mockResolvedValue(10 as any);

		const lineItems = [{ description: 'Widget', quantity: 1, unit_price: 100 }];
		const request = makeRequest({ client_id: 5, invoice_number: 'INV-001', lineItems });
		const result = await POST({ request } as any);

		expect(repositories.invoices.createInvoice).toHaveBeenCalledWith(
			expect.objectContaining({ client_id: 5, invoice_number: 'INV-001' }),
			lineItems
		);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 10 });
	});

	it('defaults lineItems to empty array', async () => {
		vi.mocked(repositories.invoices.createInvoice).mockResolvedValue(1 as any);

		const request = makeRequest({ client_id: 1 });
		await POST({ request } as any);

		expect(repositories.invoices.createInvoice).toHaveBeenCalledWith(
			expect.anything(),
			[]
		);
	});

	it('applies fkOrNull to client_id and payer_id', async () => {
		vi.mocked(repositories.invoices.createInvoice).mockResolvedValue(1 as any);

		const request = makeRequest({ client_id: 0, payer_id: '' });
		await POST({ request } as any);

		const calledWith = vi.mocked(repositories.invoices.createInvoice).mock.calls[0][0];
		expect(calledWith.client_id).toBeNull();
		expect(calledWith.payer_id).toBeNull();
	});
});

describe('GET /api/invoices/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('returns an invoice by id', () => {
		const mockInvoice = { id: 1, invoice_number: 'INV-001' };
		vi.mocked(repositories.invoices.getInvoice).mockReturnValue(mockInvoice as any);

		const result = GET_ID({ params: { id: '1' } } as any);
		expect(repositories.invoices.getInvoice).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual(mockInvoice);
	});

	it('throws 404 if invoice not found', () => {
		vi.mocked(repositories.invoices.getInvoice).mockReturnValue(undefined as any);

		try {
			GET_ID({ params: { id: '999' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(404);
			expect(e.body.message).toBe('Invoice not found');
		}
	});

	it('throws 400 for invalid id', () => {
		try {
			GET_ID({ params: { id: 'bad' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Invalid ID');
		}
	});
});

describe('PUT /api/invoices/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('updates an invoice with lineItems', async () => {
		vi.mocked(repositories.invoices.updateInvoice).mockResolvedValue(undefined as any);

		const lineItems = [{ description: 'Updated', quantity: 2, unit_price: 50 }];
		const request = makeRequest({ client_id: 3, payer_id: 7, lineItems });
		const result = await PUT({ params: { id: '1' }, request } as any);

		expect(repositories.invoices.updateInvoice).toHaveBeenCalledWith(
			1,
			expect.objectContaining({ client_id: 3, payer_id: 7 }),
			lineItems
		);
		expect((result as any).body).toEqual({ success: true });
	});

	it('defaults lineItems to empty array', async () => {
		vi.mocked(repositories.invoices.updateInvoice).mockResolvedValue(undefined as any);

		const request = makeRequest({ client_id: 1 });
		await PUT({ params: { id: '1' }, request } as any);

		expect(repositories.invoices.updateInvoice).toHaveBeenCalledWith(1, expect.anything(), []);
	});

	it('throws 400 for invalid id', async () => {
		const request = makeRequest({});
		try {
			await PUT({ params: { id: 'nope' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('DELETE /api/invoices/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('deletes an invoice', async () => {
		vi.mocked(repositories.invoices.deleteInvoice).mockResolvedValue(undefined as any);

		const result = await DELETE({ params: { id: '5' } } as any);
		expect(repositories.invoices.deleteInvoice).toHaveBeenCalledWith(5);
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

describe('PATCH /api/invoices/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('updates invoice status', async () => {
		vi.mocked(repositories.invoices.updateInvoiceStatus).mockResolvedValue(undefined as any);

		const request = makeRequest({ action: 'status', status: 'paid' });
		const result = await PATCH({ params: { id: '1' }, request } as any);

		expect(repositories.invoices.updateInvoiceStatus).toHaveBeenCalledWith(1, 'paid');
		expect((result as any).body).toEqual({ success: true });
	});

	it('duplicates an invoice', async () => {
		vi.mocked(repositories.invoices.duplicateInvoice).mockResolvedValue(99 as any);

		const request = makeRequest({ action: 'duplicate' });
		const result = await PATCH({ params: { id: '1' }, request } as any);

		expect(repositories.invoices.duplicateInvoice).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual({ id: 99 });
	});

	it('throws 400 for unknown action', async () => {
		const request = makeRequest({ action: 'unknown' });
		try {
			await PATCH({ params: { id: '1' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Unknown action');
		}
	});

	it('throws 400 for invalid id', async () => {
		const request = makeRequest({ action: 'status', status: 'paid' });
		try {
			await PATCH({ params: { id: 'abc' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Invalid ID');
		}
	});
});
