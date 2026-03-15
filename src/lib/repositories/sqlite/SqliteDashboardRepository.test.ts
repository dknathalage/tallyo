import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/dashboard.js', () => ({
	getDashboardStats: vi.fn(),
	getMonthlyRevenue: vi.fn()
}));

import { SqliteDashboardRepository } from './SqliteDashboardRepository.js';
import * as queries from '$lib/db/queries/dashboard.js';

const mockGetDashboardStats = vi.mocked(queries.getDashboardStats);
const mockGetMonthlyRevenue = vi.mocked(queries.getMonthlyRevenue);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteDashboardRepository', () => {
	describe('getDashboardStats', () => {
		it('delegates to getDashboardStats query', () => {
			const repo = new SqliteDashboardRepository();
			const stats = {
				total_invoiced: 10000,
				total_paid: 8000,
				outstanding_balance: 2000,
				overdue_count: 1,
				draft_count: 2,
				sent_count: 3
			} as any;
			mockGetDashboardStats.mockReturnValue(stats);

			const result = repo.getDashboardStats();
			expect(mockGetDashboardStats).toHaveBeenCalled();
			expect(result).toBe(stats);
		});
	});

	describe('getMonthlyRevenue', () => {
		it('delegates to getMonthlyRevenue query', () => {
			const repo = new SqliteDashboardRepository();
			const revenue = [{ month: '2025-01', total: 5000 }] as any;
			mockGetMonthlyRevenue.mockReturnValue(revenue);

			const result = repo.getMonthlyRevenue();
			expect(mockGetMonthlyRevenue).toHaveBeenCalled();
			expect(result).toBe(revenue);
		});

		it('returns empty array when no revenue', () => {
			const repo = new SqliteDashboardRepository();
			mockGetMonthlyRevenue.mockReturnValue([]);

			const result = repo.getMonthlyRevenue();
			expect(result).toEqual([]);
		});
	});
});
