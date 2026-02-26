import { query } from '../connection.svelte.js';
import type { DashboardStats, Invoice, Estimate } from '../../types/index.js';
import { getBusinessProfile } from './business-profile.js';

export function getDashboardStats(): DashboardStats {
	const profile = getBusinessProfile();
	const defaultCurrency = profile?.default_currency || 'USD';

	const revenueResult = query<{ total: number | null }>(
		`SELECT COALESCE(SUM(total), 0) as total FROM invoices WHERE status = 'paid' AND COALESCE(currency_code, 'USD') = ?`,
		[defaultCurrency]
	);
	const total_revenue = revenueResult[0]?.total ?? 0;

	const outstandingResult = query<{ total: number | null }>(
		`SELECT COALESCE(SUM(total), 0) as total FROM invoices WHERE status IN ('sent', 'overdue') AND COALESCE(currency_code, 'USD') = ?`,
		[defaultCurrency]
	);
	const outstanding_amount = outstandingResult[0]?.total ?? 0;

	const overdueResult = query<{ count: number }>(
		`SELECT COUNT(*) as count FROM invoices WHERE status = 'overdue'`
	);
	const overdue_count = overdueResult[0]?.count ?? 0;

	const clientResult = query<{ count: number }>(
		`SELECT COUNT(*) as count FROM clients`
	);
	const total_clients = clientResult[0]?.count ?? 0;

	const invoiceResult = query<{ count: number }>(
		`SELECT COUNT(*) as count FROM invoices`
	);
	const total_invoices = invoiceResult[0]?.count ?? 0;

	const excludedResult = query<{ count: number }>(
		`SELECT COUNT(*) as count FROM invoices WHERE COALESCE(currency_code, 'USD') != ?`,
		[defaultCurrency]
	);
	const excluded_currency_count = excludedResult[0]?.count ?? 0;

	const recent_invoices = query<Invoice>(
		`SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id ORDER BY i.created_at DESC LIMIT 5`
	);

	// Estimate stats - use try/catch in case table doesn't exist yet
	let total_estimates = 0;
	let pending_estimates = 0;
	let recent_estimates: Estimate[] = [];

	try {
		const estimateResult = query<{ count: number }>(
			`SELECT COUNT(*) as count FROM estimates`
		);
		total_estimates = estimateResult[0]?.count ?? 0;

		const pendingResult = query<{ count: number }>(
			`SELECT COUNT(*) as count FROM estimates WHERE status IN ('draft', 'sent')`
		);
		pending_estimates = pendingResult[0]?.count ?? 0;

		recent_estimates = query<Estimate>(
			`SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id ORDER BY e.created_at DESC LIMIT 5`
		);
	} catch {
		// estimates table may not exist yet
	}

	return {
		total_revenue,
		outstanding_amount,
		overdue_count,
		total_clients,
		total_invoices,
		excluded_currency_count,
		recent_invoices,
		total_estimates,
		pending_estimates,
		recent_estimates
	};
}
