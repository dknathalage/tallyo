import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn()
}));

vi.mock('./business-profile.js', () => ({
	getBusinessProfile: vi.fn().mockReturnValue({ default_currency: 'USD' })
}));

import { getDashboardStats } from './dashboard.js';
import { query } from '../connection.js';

const mockQuery = vi.mocked(query);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getDashboardStats', () => {
	function setupMockQueries(overrides: {
		revenue?: number;
		outstanding?: number;
		overdue?: number;
		clients?: number;
		invoices?: number;
		excludedCurrency?: number;
		recentInvoices?: unknown[];
		totalEstimates?: number;
		pendingEstimates?: number;
		recentEstimates?: unknown[];
	} = {}) {
		mockQuery
			.mockReturnValueOnce([{ total: overrides.revenue ?? 5000 }])           // revenue
			.mockReturnValueOnce([{ total: overrides.outstanding ?? 2000 }])       // outstanding
			.mockReturnValueOnce([{ count: overrides.overdue ?? 1 }])              // overdue count
			.mockReturnValueOnce([{ count: overrides.clients ?? 3 }])              // total clients
			.mockReturnValueOnce([{ count: overrides.invoices ?? 10 }])            // total invoices
			.mockReturnValueOnce([{ count: overrides.excludedCurrency ?? 0 }])     // excluded currency count
			.mockReturnValueOnce(overrides.recentInvoices ?? [])                   // recent invoices
			.mockReturnValueOnce([{ count: overrides.totalEstimates ?? 5 }])       // total estimates
			.mockReturnValueOnce([{ count: overrides.pendingEstimates ?? 2 }])     // pending estimates
			.mockReturnValueOnce(overrides.recentEstimates ?? []);                 // recent estimates
	}

	it('returns all dashboard stats', () => {
		setupMockQueries();

		const stats = getDashboardStats();

		expect(stats).toEqual({
			total_revenue: 5000,
			outstanding_amount: 2000,
			overdue_count: 1,
			total_clients: 3,
			total_invoices: 10,
			excluded_currency_count: 0,
			recent_invoices: [],
			total_estimates: 5,
			pending_estimates: 2,
			recent_estimates: []
		});
	});

	it('queries paid invoices for revenue', () => {
		setupMockQueries();
		getDashboardStats();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("status = 'paid'"),
			expect.any(Array)
		);
	});

	it('queries sent and overdue invoices for outstanding amount', () => {
		setupMockQueries();
		getDashboardStats();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("status IN ('sent', 'overdue')"),
			expect.any(Array)
		);
	});

	it('handles null/zero values gracefully', () => {
		mockQuery
			.mockReturnValueOnce([{ total: null }])      // revenue
			.mockReturnValueOnce([{ total: null }])       // outstanding
			.mockReturnValueOnce([{ count: 0 }])          // overdue count
			.mockReturnValueOnce([{ count: 0 }])          // total clients
			.mockReturnValueOnce([{ count: 0 }])          // total invoices
			.mockReturnValueOnce([{ count: 0 }])          // excluded currency count
			.mockReturnValueOnce([])                       // recent invoices
			.mockReturnValueOnce([{ count: 0 }])          // total estimates
			.mockReturnValueOnce([{ count: 0 }])          // pending estimates
			.mockReturnValueOnce([]);                      // recent estimates

		const stats = getDashboardStats();

		expect(stats.total_revenue).toBe(0);
		expect(stats.outstanding_amount).toBe(0);
		expect(stats.overdue_count).toBe(0);
		expect(stats.total_clients).toBe(0);
		expect(stats.total_invoices).toBe(0);
		expect(stats.excluded_currency_count).toBe(0);
		expect(stats.recent_invoices).toEqual([]);
		expect(stats.total_estimates).toBe(0);
		expect(stats.pending_estimates).toBe(0);
		expect(stats.recent_estimates).toEqual([]);
	});

	it('limits recent invoices to 5', () => {
		setupMockQueries();
		getDashboardStats();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('LIMIT 5')
		);
	});
});

import { getMonthlyRevenue } from './dashboard.js';

describe('getMonthlyRevenue', () => {
	it('returns 12 months of data', () => {
		// Mock returns some paid invoice rows
		mockQuery.mockReturnValueOnce([
			{ month: '2024-01', revenue: 1000 },
			{ month: '2024-03', revenue: 2500 }
		]);

		const result = getMonthlyRevenue();
		expect(result).toHaveLength(12);
		result.forEach((r) => {
			expect(r).toHaveProperty('month');
			expect(r).toHaveProperty('label');
			expect(r).toHaveProperty('revenue');
		});
	});

	it('fills missing months with 0 revenue', () => {
		mockQuery.mockReturnValueOnce([]);
		const result = getMonthlyRevenue();
		expect(result).toHaveLength(12);
		result.forEach((r) => expect(r.revenue).toBe(0));
	});
});
