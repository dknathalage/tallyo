import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn()
}));

import { getDashboardStats } from './dashboard.js';
import { query } from '../connection.svelte.js';

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
		recentInvoices?: unknown[];
	} = {}) {
		mockQuery
			.mockReturnValueOnce([{ total: overrides.revenue ?? 5000 }])       // revenue
			.mockReturnValueOnce([{ total: overrides.outstanding ?? 2000 }])   // outstanding
			.mockReturnValueOnce([{ count: overrides.overdue ?? 1 }])          // overdue count
			.mockReturnValueOnce([{ count: overrides.clients ?? 3 }])          // total clients
			.mockReturnValueOnce([{ count: overrides.invoices ?? 10 }])        // total invoices
			.mockReturnValueOnce(overrides.recentInvoices ?? []);              // recent invoices
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
			recent_invoices: []
		});
	});

	it('queries paid invoices for revenue', () => {
		setupMockQueries();
		getDashboardStats();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("status = 'paid'")
		);
	});

	it('queries sent and overdue invoices for outstanding amount', () => {
		setupMockQueries();
		getDashboardStats();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("status IN ('sent', 'overdue')")
		);
	});

	it('handles null/zero values gracefully', () => {
		mockQuery
			.mockReturnValueOnce([{ total: null }])
			.mockReturnValueOnce([{ total: null }])
			.mockReturnValueOnce([{ count: 0 }])
			.mockReturnValueOnce([{ count: 0 }])
			.mockReturnValueOnce([{ count: 0 }])
			.mockReturnValueOnce([]);

		const stats = getDashboardStats();

		expect(stats.total_revenue).toBe(0);
		expect(stats.outstanding_amount).toBe(0);
		expect(stats.overdue_count).toBe(0);
		expect(stats.total_clients).toBe(0);
		expect(stats.total_invoices).toBe(0);
		expect(stats.recent_invoices).toEqual([]);
	});

	it('limits recent invoices to 5', () => {
		setupMockQueries();
		getDashboardStats();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('LIMIT 5')
		);
	});
});
