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
		clients: {
			getClients: vi.fn(),
			getClient: vi.fn(),
			createClient: vi.fn(),
			updateClient: vi.fn(),
			deleteClient: vi.fn(),
			bulkDeleteClients: vi.fn()
		}
	}
}));

vi.mock('$lib/validation/validate.js', () => ({
	validate: (_schema: unknown, data: unknown) => data
}));

vi.mock('$lib/validation/schemas.js', () => ({
	CreateClientSchema: {},
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

describe('GET /api/clients', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('calls getClients with search and pagination', async () => {
		const mockData = { data: [], total: 0 };
		vi.mocked(repositories.clients.getClients).mockResolvedValue(mockData as any);

		const url = makeUrl('/api/clients', { search: 'acme', page: '2', limit: '25' });
		const result = await GET({ url } as any);

		expect(repositories.clients.getClients).toHaveBeenCalledWith('acme', { page: 2, limit: 25 });
		expect((result as any).body).toEqual(mockData);
	});

	it('calls getClients with defaults when no params', async () => {
		vi.mocked(repositories.clients.getClients).mockResolvedValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/clients');
		await GET({ url } as any);

		expect(repositories.clients.getClients).toHaveBeenCalledWith(undefined, { page: 1, limit: 50 });
	});

	it('caps limit at 200', async () => {
		vi.mocked(repositories.clients.getClients).mockResolvedValue({ data: [], total: 0 } as any);

		const url = makeUrl('/api/clients', { limit: '999' });
		await GET({ url } as any);

		expect(repositories.clients.getClients).toHaveBeenCalledWith(undefined, { page: 1, limit: 200 });
	});

	it('rejects search longer than 255 chars', async () => {
		const longSearch = 'a'.repeat(256);
		const url = makeUrl('/api/clients', { search: longSearch });

		try {
			await GET({ url } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Search query too long');
		}
	});
});

describe('POST /api/clients', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('creates a client and returns 201', async () => {
		vi.mocked(repositories.clients.createClient).mockResolvedValue(42 as any);

		const request = makeRequest({ name: 'Acme Corp', email: 'a@b.com' });
		const result = await POST({ request } as any);

		expect(repositories.clients.createClient).toHaveBeenCalled();
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 42 });
	});

	it('handles fkOrNull for pricing_tier_id and payer_id', async () => {
		vi.mocked(repositories.clients.createClient).mockResolvedValue(1 as any);

		const request = makeRequest({ name: 'Test', pricing_tier_id: 0, payer_id: '' });
		await POST({ request } as any);

		const calledWith = vi.mocked(repositories.clients.createClient).mock.calls[0]?.[0];
		expect(calledWith?.pricing_tier_id).toBeNull();
		expect(calledWith?.payer_id).toBeNull();
	});

	it('bulk-deletes clients', async () => {
		vi.mocked(repositories.clients.bulkDeleteClients).mockResolvedValue(undefined as any);

		const request = makeRequest({ action: 'bulk-delete', ids: [1, 2, 3] });
		const result = await POST({ request } as any);

		expect(repositories.clients.bulkDeleteClients).toHaveBeenCalledWith([1, 2, 3]);
		expect((result as any).body).toEqual({ success: true });
	});
});

describe('GET /api/clients/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('returns a client by id', async () => {
		const mockClient = { id: 1, name: 'Acme' };
		vi.mocked(repositories.clients.getClient).mockResolvedValue(mockClient as any);

		const result = await GET_ID({ params: { id: '1' } } as any);
		expect(repositories.clients.getClient).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual(mockClient);
	});

	it('throws 404 if client not found', async () => {
		vi.mocked(repositories.clients.getClient).mockResolvedValue(undefined as any);

		try {
			await GET_ID({ params: { id: '1' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(404);
			expect(e.body.message).toBe('Client not found');
		}
	});

	it('throws 400 for invalid id', async () => {
		try {
			await GET_ID({ params: { id: 'abc' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Invalid ID');
		}
	});

	it('throws 400 for negative id', async () => {
		try {
			await GET_ID({ params: { id: '-5' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('throws 400 for zero id', async () => {
		try {
			await GET_ID({ params: { id: '0' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('PUT /api/clients/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('updates a client', async () => {
		vi.mocked(repositories.clients.updateClient).mockResolvedValue(undefined as any);

		const request = makeRequest({ name: 'Updated', pricing_tier_id: 5, payer_id: 3 });
		const result = await PUT({ params: { id: '1' }, request } as any);

		expect(repositories.clients.updateClient).toHaveBeenCalledWith(1, expect.objectContaining({
			name: 'Updated',
			pricing_tier_id: 5,
			payer_id: 3
		}));
		expect((result as any).body).toEqual({ success: true });
	});

	it('throws 400 for invalid id', async () => {
		const request = makeRequest({ name: 'Test' });
		try {
			await PUT({ params: { id: 'bad' }, request } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});

describe('DELETE /api/clients/[id]', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('deletes a client', async () => {
		vi.mocked(repositories.clients.deleteClient).mockResolvedValue(undefined as any);

		const result = await DELETE({ params: { id: '1' } } as any);
		expect(repositories.clients.deleteClient).toHaveBeenCalledWith(1);
		expect((result as any).body).toEqual({ success: true });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await DELETE({ params: { id: 'xyz' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});
});
