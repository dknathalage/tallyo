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
		catalog: {
			getCatalogItems: vi.fn(),
			getCatalogItem: vi.fn(),
			createCatalogItem: vi.fn(),
			updateCatalogItem: vi.fn(),
			deleteCatalogItem: vi.fn(),
			bulkDeleteCatalogItems: vi.fn()
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
import { repositories } from '$lib/repositories/sqlite/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}
function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/catalog', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns catalog items with defaults', () => {
		const items = [{ id: 1, name: 'Widget' }];
		vi.mocked(repositories.catalog.getCatalogItems).mockReturnValue(items as any);

		const result = GET({ url: makeUrl('/api/catalog') } as any);
		expect((result as any).body).toEqual(items);
		expect(repositories.catalog.getCatalogItems).toHaveBeenCalledWith(undefined, undefined, { page: 1, limit: 50 });
	});

	it('passes search and category params', () => {
		vi.mocked(repositories.catalog.getCatalogItems).mockReturnValue([] as any);

		GET({ url: makeUrl('/api/catalog', { search: 'test', category: 'parts', page: '2', limit: '10' }) } as any);
		expect(repositories.catalog.getCatalogItems).toHaveBeenCalledWith('test', 'parts', { page: 2, limit: 10 });
	});

	it('caps limit at 200', () => {
		vi.mocked(repositories.catalog.getCatalogItems).mockReturnValue([] as any);

		GET({ url: makeUrl('/api/catalog', { limit: '999' }) } as any);
		expect(repositories.catalog.getCatalogItems).toHaveBeenCalledWith(undefined, undefined, { page: 1, limit: 200 });
	});

	it('throws 400 if search too long', () => {
		const longSearch = 'a'.repeat(256);
		try {
			GET({ url: makeUrl('/api/catalog', { search: longSearch }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Search query too long');
		}
	});
});

describe('POST /api/catalog', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a catalog item', async () => {
		vi.mocked(repositories.catalog.createCatalogItem).mockResolvedValue(42 as any);
		const result = await POST({ request: makeRequest({ name: 'Widget', price: 10 }) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 42 });
	});

	it('handles bulk-delete action', async () => {
		vi.mocked(repositories.catalog.bulkDeleteCatalogItems).mockResolvedValue(undefined as any);
		const result = await POST({ request: makeRequest({ action: 'bulk-delete', ids: [1, 2, 3] }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.catalog.bulkDeleteCatalogItems).toHaveBeenCalledWith([1, 2, 3]);
	});

	it('propagates db errors on create', async () => {
		vi.mocked(repositories.catalog.createCatalogItem).mockRejectedValue(new Error('DB fail'));
		await expect(POST({ request: makeRequest({ name: 'Bad' }) } as any)).rejects.toThrow('DB fail');
	});

	it('propagates db errors on bulk-delete', async () => {
		vi.mocked(repositories.catalog.bulkDeleteCatalogItems).mockRejectedValue(new Error('FK error'));
		await expect(POST({ request: makeRequest({ action: 'bulk-delete', ids: [1] }) } as any)).rejects.toThrow('FK error');
	});
});

describe('GET /api/catalog/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns item by id', () => {
		const item = { id: 1, name: 'Widget' };
		vi.mocked(repositories.catalog.getCatalogItem).mockReturnValue(item as any);
		const result = GET_ID({ params: { id: '1' } } as any);
		expect((result as any).body).toEqual(item);
	});

	it('throws 404 if not found', () => {
		vi.mocked(repositories.catalog.getCatalogItem).mockReturnValue(undefined as any);
		try {
			GET_ID({ params: { id: '999' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(404);
			expect(e.body.message).toBe('Catalog item not found');
		}
	});

	it('throws 400 for invalid id', () => {
		try {
			GET_ID({ params: { id: 'abc' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Invalid ID');
		}
	});

	it('throws 400 for non-positive id', () => {
		try {
			GET_ID({ params: { id: '0' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('PUT /api/catalog/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('updates item', async () => {
		vi.mocked(repositories.catalog.updateCatalogItem).mockResolvedValue(undefined as any);
		const result = await PUT({ params: { id: '1' }, request: makeRequest({ name: 'Updated' }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.catalog.updateCatalogItem).toHaveBeenCalledWith(1, { name: 'Updated' });
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
		vi.mocked(repositories.catalog.updateCatalogItem).mockRejectedValue(new Error('DB error'));
		await expect(PUT({ params: { id: '1' }, request: makeRequest({}) } as any)).rejects.toThrow('DB error');
	});
});

describe('DELETE /api/catalog/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes item', async () => {
		vi.mocked(repositories.catalog.deleteCatalogItem).mockResolvedValue(undefined as any);
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
		vi.mocked(repositories.catalog.deleteCatalogItem).mockRejectedValue(new Error('FK constraint'));
		await expect(DELETE({ params: { id: '1' } } as any)).rejects.toThrow('FK constraint');
	});
});
