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
		columnMappings: {
			getColumnMappings: vi.fn(),
			createColumnMapping: vi.fn(),
			deleteColumnMapping: vi.fn()
		}
	}
}));

import { GET, POST, DELETE } from './+server.js';
import { repositories } from '$lib/repositories/sqlite/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}
function makeUrl(base: string, params?: Record<string, string>) {
	const url = new URL(base, 'http://localhost');
	if (params) Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
	return url;
}

describe('GET /api/column-mappings', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns mappings for default entity (catalog)', () => {
		const mappings = [{ id: 1, source: 'col_a', target: 'name' }];
		vi.mocked(repositories.columnMappings.getColumnMappings).mockReturnValue(mappings as any);
		const result = GET({ url: makeUrl('/api/column-mappings') } as any);
		expect((result as any).body).toEqual(mappings);
		expect(repositories.columnMappings.getColumnMappings).toHaveBeenCalledWith('catalog');
	});

	it('returns mappings for specified entity', () => {
		vi.mocked(repositories.columnMappings.getColumnMappings).mockReturnValue([] as any);
		GET({ url: makeUrl('/api/column-mappings', { entity: 'invoices' }) } as any);
		expect(repositories.columnMappings.getColumnMappings).toHaveBeenCalledWith('invoices');
	});
});

describe('POST /api/column-mappings', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a column mapping', async () => {
		vi.mocked(repositories.columnMappings.createColumnMapping).mockResolvedValue(5 as any);
		const data = { entity: 'catalog', source: 'col_a', target: 'name' };
		const result = await POST({ request: makeRequest(data) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 5 });
		expect(repositories.columnMappings.createColumnMapping).toHaveBeenCalledWith(data);
	});
});

describe('DELETE /api/column-mappings', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes a column mapping by id param', async () => {
		vi.mocked(repositories.columnMappings.deleteColumnMapping).mockResolvedValue(undefined as any);
		const result = await DELETE({ url: makeUrl('/api/column-mappings', { id: '3' }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.columnMappings.deleteColumnMapping).toHaveBeenCalledWith(3);
	});

	it('passes 0 when id param is missing', async () => {
		vi.mocked(repositories.columnMappings.deleteColumnMapping).mockResolvedValue(undefined as any);
		const result = await DELETE({ url: makeUrl('/api/column-mappings') } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.columnMappings.deleteColumnMapping).toHaveBeenCalledWith(0);
	});
});
