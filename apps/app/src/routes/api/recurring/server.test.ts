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

vi.mock('$lib/repositories/postgres/index.js', () => ({
	repositories: {
		recurringTemplates: {
			getRecurringTemplates: vi.fn(),
			getRecurringTemplate: vi.fn(),
			createRecurringTemplate: vi.fn(),
			updateRecurringTemplate: vi.fn(),
			deleteRecurringTemplate: vi.fn(),
			createInvoiceFromTemplate: vi.fn()
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
import { GET as GET_ID, PUT, DELETE, PATCH } from './[id]/+server.js';
import { repositories } from '$lib/repositories/postgres/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}
function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/recurring', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns active templates by default', async () => {
		const templates = [{ id: 1, name: 'Monthly' }];
		vi.mocked(repositories.recurringTemplates.getRecurringTemplates).mockResolvedValue(templates as any);
		const result = await GET({ url: makeUrl('/api/recurring') } as any);
		expect((result as any).body).toEqual(templates);
		expect(repositories.recurringTemplates.getRecurringTemplates).toHaveBeenCalledWith(true);
	});

	it('returns all templates when all=true', async () => {
		vi.mocked(repositories.recurringTemplates.getRecurringTemplates).mockResolvedValue([] as any);
		await GET({ url: makeUrl('/api/recurring', { all: 'true' }) } as any);
		expect(repositories.recurringTemplates.getRecurringTemplates).toHaveBeenCalledWith(false);
	});
});

describe('POST /api/recurring', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a recurring template', async () => {
		vi.mocked(repositories.recurringTemplates.createRecurringTemplate).mockResolvedValue(8 as any);
		const result = await POST({ request: makeRequest({ name: 'Weekly', interval: 'weekly' }) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 8 });
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.recurringTemplates.createRecurringTemplate).mockRejectedValue(new Error('DB fail'));
		await expect(POST({ request: makeRequest({ name: 'Bad' }) } as any)).rejects.toThrow('DB fail');
	});
});

describe('GET /api/recurring/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns template by id', async () => {
		const template = { id: 1, name: 'Monthly' };
		vi.mocked(repositories.recurringTemplates.getRecurringTemplate).mockResolvedValue(template as any);
		const result = await GET_ID({ params: { id: '1' } } as any);
		expect((result as any).body).toEqual(template);
	});

	it('throws 404 if not found', async () => {
		vi.mocked(repositories.recurringTemplates.getRecurringTemplate).mockResolvedValue(undefined as any);
		try {
			await GET_ID({ params: { id: '999' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(404);
			expect(e.body.message).toBe('Template not found');
		}
	});

	it('throws 400 for invalid id', async () => {
		try {
			await GET_ID({ params: { id: 'abc' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('PUT /api/recurring/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('updates a template', async () => {
		vi.mocked(repositories.recurringTemplates.updateRecurringTemplate).mockResolvedValue(undefined as any);
		const result = await PUT({ params: { id: '1' }, request: makeRequest({ name: 'Updated' }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.recurringTemplates.updateRecurringTemplate).toHaveBeenCalledWith(1, { name: 'Updated' });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await PUT({ params: { id: '-1' }, request: makeRequest({}) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.recurringTemplates.updateRecurringTemplate).mockRejectedValue(new Error('DB error'));
		await expect(PUT({ params: { id: '1' }, request: makeRequest({}) } as any)).rejects.toThrow('DB error');
	});
});

describe('DELETE /api/recurring/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes a template', async () => {
		vi.mocked(repositories.recurringTemplates.deleteRecurringTemplate).mockResolvedValue(undefined as any);
		const result = await DELETE({ params: { id: '1' } } as any);
		expect((result as any).body).toEqual({ success: true });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await DELETE({ params: { id: 'abc' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.recurringTemplates.deleteRecurringTemplate).mockRejectedValue(new Error('FK constraint'));
		await expect(DELETE({ params: { id: '1' } } as any)).rejects.toThrow('FK constraint');
	});
});

describe('PATCH /api/recurring/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates invoice from template', async () => {
		vi.mocked(repositories.recurringTemplates.createInvoiceFromTemplate).mockResolvedValue(42 as any);
		const result = await PATCH({ params: { id: '1' } } as any);
		expect((result as any).body).toEqual({ invoiceId: 42 });
		expect(repositories.recurringTemplates.createInvoiceFromTemplate).toHaveBeenCalledWith(1);
	});

	it('throws 400 for invalid id', async () => {
		try {
			await PATCH({ params: { id: '0' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('propagates db errors', async () => {
		vi.mocked(repositories.recurringTemplates.createInvoiceFromTemplate).mockRejectedValue(new Error('Template error'));
		await expect(PATCH({ params: { id: '1' } } as any)).rejects.toThrow('Template error');
	});
});
