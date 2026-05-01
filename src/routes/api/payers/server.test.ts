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
		payers: {
			getPayers: vi.fn(),
			getPayer: vi.fn(),
			createPayer: vi.fn(),
			updatePayer: vi.fn(),
			deletePayer: vi.fn(),
			bulkDeletePayers: vi.fn()
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
	BulkDeleteSchema: {},
	SearchParamsSchema: {}
}));

import { GET, POST } from './+server.js';
import { GET as GET_ID, PUT, DELETE } from './[id]/+server.js';
import { repositories } from '$lib/repositories/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}
function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/payers', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns payers without search', async () => {
		const payers = [{ id: 1, name: 'Acme' }];
		vi.mocked(repositories.payers.getPayers).mockResolvedValue(payers as any);
		const result = await GET({ url: makeUrl('/api/payers') } as any);
		expect((result as any).body).toEqual(payers);
		expect(repositories.payers.getPayers).toHaveBeenCalledWith(undefined);
	});

	it('passes search param', async () => {
		vi.mocked(repositories.payers.getPayers).mockResolvedValue([]);
		await GET({ url: makeUrl('/api/payers', { search: 'acme' }) } as any);
		expect(repositories.payers.getPayers).toHaveBeenCalledWith('acme');
	});

	it('throws 400 if search too long', async () => {
		const longSearch = 'a'.repeat(256);
		try {
			await GET({ url: makeUrl('/api/payers', { search: longSearch }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Search query too long');
		}
	});
});

describe('POST /api/payers', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a payer', async () => {
		vi.mocked(repositories.payers.createPayer).mockResolvedValue(5);
		const result = await POST({ request: makeRequest({ name: 'Acme Corp' }) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 5 });
	});

	it('handles bulk-delete action', async () => {
		vi.mocked(repositories.payers.bulkDeletePayers).mockResolvedValue(undefined);
		const result = await POST({ request: makeRequest({ action: 'bulk-delete', ids: [1, 2] }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.payers.bulkDeletePayers).toHaveBeenCalledWith([1, 2]);
	});

	it('propagates db errors on create', async () => {
		vi.mocked(repositories.payers.createPayer).mockRejectedValue(new Error('DB fail'));
		await expect(POST({ request: makeRequest({ name: 'Bad' }) } as any)).rejects.toThrow('DB fail');
	});

	it('propagates db errors on bulk-delete', async () => {
		vi.mocked(repositories.payers.bulkDeletePayers).mockRejectedValue(new Error('FK error'));
		await expect(POST({ request: makeRequest({ action: 'bulk-delete', ids: [1] }) } as any)).rejects.toThrow('FK error');
	});
});

describe('GET /api/payers/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns payer by id', async () => {
		const payer = { id: 1, name: 'Acme' };
		vi.mocked(repositories.payers.getPayer).mockResolvedValue(payer as any);
		const result = await GET_ID({ params: { id: '1' } } as any);
		expect((result as any).body).toEqual(payer);
	});

	it('throws 404 if not found', async () => {
		vi.mocked(repositories.payers.getPayer).mockResolvedValue(undefined as any);
		try {
			await GET_ID({ params: { id: '999' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(404);
			expect(e.body.message).toBe('Payer not found');
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

describe('PUT /api/payers/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('updates payer', async () => {
		vi.mocked(repositories.payers.updatePayer).mockResolvedValue(undefined);
		const result = await PUT({ params: { id: '1' }, request: makeRequest({ name: 'Updated' }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.payers.updatePayer).toHaveBeenCalledWith(1, { name: 'Updated' });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await PUT({ params: { id: '-1' }, request: makeRequest({}) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('DELETE /api/payers/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes payer', async () => {
		vi.mocked(repositories.payers.deletePayer).mockResolvedValue(undefined);
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
		vi.mocked(repositories.payers.deletePayer).mockRejectedValue(new Error('FK constraint'));
		await expect(DELETE({ params: { id: '1' } } as any)).rejects.toThrow('FK constraint');
	});
});
