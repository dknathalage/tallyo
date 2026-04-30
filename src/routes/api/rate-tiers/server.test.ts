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
		rateTiers: {
			getRateTiers: vi.fn(),
			createRateTier: vi.fn(),
			updateRateTier: vi.fn(),
			deleteRateTier: vi.fn()
		}
	}
}));

import { GET, POST } from './+server.js';
import { PUT, DELETE } from './[id]/+server.js';
import { repositories } from '$lib/repositories/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}

describe('GET /api/rate-tiers', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns list of rate tiers', async () => {
		const tiers = [{ id: 1, name: 'Standard' }];
		vi.mocked(repositories.rateTiers.getRateTiers).mockResolvedValue(tiers as any);
		const result = await GET({} as any);
		expect((result as any).body).toEqual(tiers);
	});
});

describe('POST /api/rate-tiers', () => {
	beforeEach(() => vi.clearAllMocks());

	it('creates a rate tier', async () => {
		vi.mocked(repositories.rateTiers.createRateTier).mockResolvedValue(3 as any);
		const result = await POST({ request: makeRequest({ name: 'Premium' }) } as any);
		expect((result as any).status).toBe(201);
		expect((result as any).body).toEqual({ id: 3 });
	});

	it('throws 400 when name is missing', async () => {
		try {
			await POST({ request: makeRequest({}) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Tier name is required');
		}
	});

	it('throws 400 when name is empty string', async () => {
		try {
			await POST({ request: makeRequest({ name: '  ' }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Tier name is required');
		}
	});

	it('throws 409 on UNIQUE constraint failure', async () => {
		vi.mocked(repositories.rateTiers.createRateTier).mockRejectedValue(new Error('UNIQUE constraint failed: rate_tiers.name'));
		try {
			await POST({ request: makeRequest({ name: 'Duplicate' }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(409);
			expect(e.body.message).toBe('A rate tier named "Duplicate" already exists');
		}
	});

	it('rethrows non-unique errors', async () => {
		vi.mocked(repositories.rateTiers.createRateTier).mockRejectedValue(new Error('Some other error'));
		await expect(POST({ request: makeRequest({ name: 'Valid' }) } as any)).rejects.toThrow('Some other error');
	});
});

describe('PUT /api/rate-tiers/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('updates a rate tier', async () => {
		vi.mocked(repositories.rateTiers.updateRateTier).mockResolvedValue(undefined as any);
		const result = await PUT({ params: { id: '1' }, request: makeRequest({ name: 'Updated' }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.rateTiers.updateRateTier).toHaveBeenCalledWith(1, { name: 'Updated' });
	});

	it('throws 400 for invalid id', async () => {
		try {
			await PUT({ params: { id: 'abc' }, request: makeRequest({ name: 'X' }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
		}
	});

	it('throws 400 when name is empty', async () => {
		try {
			await PUT({ params: { id: '1' }, request: makeRequest({ name: '' }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Tier name is required');
		}
	});

	it('throws 409 on UNIQUE constraint failure', async () => {
		vi.mocked(repositories.rateTiers.updateRateTier).mockRejectedValue(new Error('UNIQUE constraint failed'));
		try {
			await PUT({ params: { id: '1' }, request: makeRequest({ name: 'Dup' }) } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(409);
			expect(e.body.message).toBe('A rate tier named "Dup" already exists');
		}
	});

	it('rethrows non-unique errors', async () => {
		vi.mocked(repositories.rateTiers.updateRateTier).mockRejectedValue(new Error('Other error'));
		await expect(PUT({ params: { id: '1' }, request: makeRequest({ name: 'Valid' }) } as any)).rejects.toThrow('Other error');
	});
});

describe('DELETE /api/rate-tiers/[id]', () => {
	beforeEach(() => vi.clearAllMocks());

	it('deletes a rate tier', async () => {
		vi.mocked(repositories.rateTiers.deleteRateTier).mockResolvedValue(undefined as any);
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

	it('throws 400 when trying to delete last tier', async () => {
		vi.mocked(repositories.rateTiers.deleteRateTier).mockRejectedValue(new Error('Cannot delete the last tier'));
		try {
			await DELETE({ params: { id: '1' } } as any);
			expect.unreachable('should have thrown');
		} catch (e: any) {
			expect(e.status).toBe(400);
			expect(e.body.message).toBe('Cannot delete the last tier');
		}
	});

	it('rethrows non-last-tier errors', async () => {
		vi.mocked(repositories.rateTiers.deleteRateTier).mockRejectedValue(new Error('FK constraint'));
		await expect(DELETE({ params: { id: '1' } } as any)).rejects.toThrow('FK constraint');
	});
});
