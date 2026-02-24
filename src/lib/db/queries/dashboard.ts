import { query } from '../connection.svelte.js';
import type { DashboardStats, Invoice } from '../../types/index.js';

export function getDashboardStats(): DashboardStats {
	const revenueResult = query<{ total: number | null }>(
		`SELECT COALESCE(SUM(total), 0) as total FROM invoices WHERE status = 'paid'`
	);
	const total_revenue = revenueResult[0]?.total ?? 0;

	const outstandingResult = query<{ total: number | null }>(
		`SELECT COALESCE(SUM(total), 0) as total FROM invoices WHERE status IN ('sent', 'overdue')`
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

	const recent_invoices = query<Invoice>(
		`SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id ORDER BY i.created_at DESC LIMIT 5`
	);

	return {
		total_revenue,
		outstanding_amount,
		overdue_count,
		total_clients,
		total_invoices,
		recent_invoices
	};
}
