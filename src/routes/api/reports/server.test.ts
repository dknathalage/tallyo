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
			getAgingReport: vi.fn()
		},
		businessProfile: {
			getBusinessProfile: vi.fn()
		}
	}
}));

import { GET } from './+server.js';
import { repositories } from '$lib/repositories/sqlite/index.js';

describe('GET /api/reports', () => {
	beforeEach(() => vi.clearAllMocks());

	it('returns agingBuckets and defaultCurrency', () => {
		const buckets = [{ bucket: '0-30', total: 500 }];
		vi.mocked(repositories.invoices.getAgingReport).mockReturnValue(buckets as any);
		vi.mocked(repositories.businessProfile.getBusinessProfile).mockReturnValue({ default_currency: 'EUR' } as any);

		const result = GET({} as any);
		expect((result as any).body).toEqual({ agingBuckets: buckets, defaultCurrency: 'EUR' });
	});

	it('defaults to USD when profile is missing', () => {
		vi.mocked(repositories.invoices.getAgingReport).mockReturnValue([] as any);
		vi.mocked(repositories.businessProfile.getBusinessProfile).mockReturnValue(null as any);

		const result = GET({} as any);
		expect((result as any).body).toEqual({ agingBuckets: [], defaultCurrency: 'USD' });
	});

	it('defaults to USD when default_currency is not set', () => {
		vi.mocked(repositories.invoices.getAgingReport).mockReturnValue([] as any);
		vi.mocked(repositories.businessProfile.getBusinessProfile).mockReturnValue({} as any);

		const result = GET({} as any);
		expect((result as any).body).toEqual({ agingBuckets: [], defaultCurrency: 'USD' });
	});
});
