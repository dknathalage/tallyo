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
		businessProfile: {
			getBusinessProfile: vi.fn(),
			saveBusinessProfile: vi.fn()
		},
		taxRates: {
			getTaxRates: vi.fn()
		},
		columnMappings: {
			getColumnMappings: vi.fn()
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
import { repositories } from '$lib/repositories/index.js';

function makeRequest(body: unknown) {
	return { json: async () => body } as unknown as Request;
}

describe('GET /api/settings', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns combined profile, taxRates, and columnMappings', async () => {
		const profile = { business_name: 'Test Co' };
		const taxRates = [{ id: 1, rate: 10 }];
		const mappings = [{ id: 1, entity: 'catalog' }];
		vi.mocked(repositories.businessProfile.getBusinessProfile).mockResolvedValue(profile as any);
		vi.mocked(repositories.taxRates.getTaxRates).mockResolvedValue(taxRates as any);
		vi.mocked(repositories.columnMappings.getColumnMappings).mockResolvedValue(mappings as any);

		const result = await GET({} as any);
		expect((result as any).body).toEqual({ profile, taxRates, columnMappings: mappings });
	});
});

describe('POST /api/settings', () => {
	beforeEach(() => vi.clearAllMocks());

	it('saves profile data when provided', async () => {
		vi.mocked(repositories.businessProfile.saveBusinessProfile).mockResolvedValue(undefined);
		const profileData = { business_name: 'New Name' };
		const result = await POST({ request: makeRequest({ profile: profileData }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.businessProfile.saveBusinessProfile).toHaveBeenCalledWith(profileData);
	});

	it('does not save profile when not provided', async () => {
		const result = await POST({ request: makeRequest({ other: 'data' }) } as any);
		expect((result as any).body).toEqual({ success: true });
		expect(repositories.businessProfile.saveBusinessProfile).not.toHaveBeenCalled();
	});

	it('propagates db errors on profile save', async () => {
		vi.mocked(repositories.businessProfile.saveBusinessProfile).mockRejectedValue(new Error('DB fail'));
		await expect(POST({ request: makeRequest({ profile: { name: 'X' } }) } as any)).rejects.toThrow('DB fail');
	});
});
