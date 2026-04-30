import { getDb } from '../connection.js';
import { invoices, estimates, clients } from '../drizzle-schema.js';
import { eq, sql, desc, inArray } from 'drizzle-orm';
import type { DashboardStats, Invoice, Estimate, MonthlyRevenue } from '../../types/index.js';
import { getBusinessProfile } from './business-profile.js';

function toISOString(d: string | null | undefined): string {
	if (!d) return '';
	return d;
}

function mapRowToInvoice(row: Record<string, unknown>): Invoice {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		invoice_number: row.invoice_number as string,
		client_id: row.client_id as number,
		client_name: (row.client_name as string) ?? undefined,
		date: row.date as string,
		due_date: row.due_date as string,
		payment_terms: row.payment_terms as Invoice['payment_terms'],
		subtotal: row.subtotal as number,
		tax_rate: row.tax_rate as number,
		tax_rate_id: (row.tax_rate_id as number | null) ?? null,
		tax_amount: row.tax_amount as number,
		total: row.total as number,
		notes: (row.notes as string) ?? '',
		status: row.status as Invoice['status'],
		currency_code: (row.currency_code as string) ?? 'USD',
		business_snapshot: (row.business_snapshot as string) ?? '{}',
		client_snapshot: (row.client_snapshot as string) ?? '{}',
		payer_snapshot: (row.payer_snapshot as string) ?? '{}',
		created_at: toISOString(row.created_at as string | null),
		updated_at: toISOString(row.updated_at as string | null)
	};
}

function mapRowToEstimate(row: Record<string, unknown>): Estimate {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		estimate_number: row.estimate_number as string,
		client_id: row.client_id as number,
		client_name: (row.client_name as string) ?? undefined,
		date: row.date as string,
		valid_until: row.valid_until as string,
		subtotal: row.subtotal as number,
		tax_rate: row.tax_rate as number,
		tax_rate_id: (row.tax_rate_id as number | null) ?? null,
		tax_amount: row.tax_amount as number,
		total: row.total as number,
		notes: (row.notes as string) ?? '',
		status: row.status as Estimate['status'],
		currency_code: (row.currency_code as string) ?? 'USD',
		converted_invoice_id: (row.converted_invoice_id as number | null) ?? null,
		business_snapshot: (row.business_snapshot as string) ?? '{}',
		client_snapshot: (row.client_snapshot as string) ?? '{}',
		payer_snapshot: (row.payer_snapshot as string) ?? '{}',
		created_at: toISOString(row.created_at as string | null),
		updated_at: toISOString(row.updated_at as string | null)
	};
}

export async function getDashboardStats(): Promise<DashboardStats> {
	const db = getDb();
	const profile = await getBusinessProfile();
	const defaultCurrency = profile?.default_currency || 'USD';

	const revenueResult = await db
		.select({ total: sql<number>`COALESCE(SUM(${invoices.total}), 0)` })
		.from(invoices)
		.where(
			sql`${invoices.status} = 'paid' AND COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}`
		);
	const total_revenue = revenueResult[0]?.total ?? 0;

	const outstandingResult = await db
		.select({ total: sql<number>`COALESCE(SUM(${invoices.total}), 0)` })
		.from(invoices)
		.where(
			sql`${invoices.status} IN ('sent', 'overdue') AND COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}`
		);
	const outstanding_amount = outstandingResult[0]?.total ?? 0;

	const overdueResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(invoices)
		.where(eq(invoices.status, 'overdue'));
	const overdue_count = overdueResult[0]?.count ?? 0;

	const clientResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(clients);
	const total_clients = clientResult[0]?.count ?? 0;

	const invoiceResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(invoices);
	const total_invoices = invoiceResult[0]?.count ?? 0;

	const excludedResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(invoices)
		.where(sql`COALESCE(${invoices.currency_code}, 'USD') != ${defaultCurrency}`);
	const excluded_currency_count = excludedResult[0]?.count ?? 0;

	const recentInvoiceRows = await db
		.select({
			id: invoices.id,
			uuid: invoices.uuid,
			invoice_number: invoices.invoice_number,
			client_id: invoices.client_id,
			client_name: clients.name,
			date: invoices.date,
			due_date: invoices.due_date,
			payment_terms: invoices.payment_terms,
			subtotal: invoices.subtotal,
			tax_rate: invoices.tax_rate,
			tax_rate_id: invoices.tax_rate_id,
			tax_amount: invoices.tax_amount,
			total: invoices.total,
			notes: invoices.notes,
			status: invoices.status,
			currency_code: invoices.currency_code,
			business_snapshot: invoices.business_snapshot,
			client_snapshot: invoices.client_snapshot,
			payer_snapshot: invoices.payer_snapshot,
			created_at: invoices.created_at,
			updated_at: invoices.updated_at
		})
		.from(invoices)
		.leftJoin(clients, eq(invoices.client_id, clients.id))
		.orderBy(desc(invoices.created_at))
		.limit(5);
	const recent_invoices = recentInvoiceRows.map((r) =>
		mapRowToInvoice(r as unknown as Record<string, unknown>)
	);

	// Estimate stats
	const estimateResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(estimates);
	const total_estimates = estimateResult[0]?.count ?? 0;

	const pendingResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(estimates)
		.where(inArray(estimates.status, ['draft', 'sent']));
	const pending_estimates = pendingResult[0]?.count ?? 0;

	const recentEstimateRows = await db
		.select({
			id: estimates.id,
			uuid: estimates.uuid,
			estimate_number: estimates.estimate_number,
			client_id: estimates.client_id,
			client_name: clients.name,
			date: estimates.date,
			valid_until: estimates.valid_until,
			subtotal: estimates.subtotal,
			tax_rate: estimates.tax_rate,
			tax_rate_id: estimates.tax_rate_id,
			tax_amount: estimates.tax_amount,
			total: estimates.total,
			notes: estimates.notes,
			status: estimates.status,
			currency_code: estimates.currency_code,
			converted_invoice_id: estimates.converted_invoice_id,
			business_snapshot: estimates.business_snapshot,
			client_snapshot: estimates.client_snapshot,
			payer_snapshot: estimates.payer_snapshot,
			created_at: estimates.created_at,
			updated_at: estimates.updated_at
		})
		.from(estimates)
		.leftJoin(clients, eq(estimates.client_id, clients.id))
		.orderBy(desc(estimates.created_at))
		.limit(5);
	const recent_estimates = recentEstimateRows.map((r) =>
		mapRowToEstimate(r as unknown as Record<string, unknown>)
	);

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

export async function getMonthlyRevenue(): Promise<MonthlyRevenue[]> {
	const db = getDb();
	const profile = await getBusinessProfile();
	const defaultCurrency = profile?.default_currency || 'USD';

	// Generate the last 12 months as YYYY-MM strings
	const months: string[] = [];
	const now = new Date();
	for (let i = 11; i >= 0; i--) {
		const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
		const yyyy = d.getFullYear();
		const mm = String(d.getMonth() + 1).padStart(2, '0');
		months.push(`${yyyy}-${mm}`);
	}

	const rows = await db
		.select({
			month: sql<string>`strftime('%Y-%m', ${invoices.date})`,
			revenue: sql<number>`COALESCE(SUM(${invoices.total}), 0)`
		})
		.from(invoices)
		.where(
			sql`${invoices.status} = 'paid'
				AND COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}
				AND strftime('%Y-%m', ${invoices.date}) >= strftime('%Y-%m', date('now', '-11 months'))`
		)
		.groupBy(sql`strftime('%Y-%m', ${invoices.date})`)
		.orderBy(sql`strftime('%Y-%m', ${invoices.date})`);

	const revenueMap = new Map<string, number>();
	for (const row of rows) {
		revenueMap.set(row.month, row.revenue);
	}

	return months.map((month) => {
		const [year, mon] = month.split('-');
		const label = new Date(Number(year), Number(mon) - 1, 1).toLocaleString('default', {
			month: 'short',
			year: 'numeric'
		});
		return {
			month,
			label,
			revenue: revenueMap.get(month) ?? 0
		};
	});
}
