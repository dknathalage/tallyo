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
	const clientName = row['client_name'] as string | null | undefined;
	return {
		id: row['id'] as number,
		uuid: row['uuid'] as string,
		invoice_number: row['invoice_number'] as string,
		client_id: row['client_id'] as number,
		...(clientName !== null && clientName !== undefined ? { client_name: clientName } : {}),
		date: row['date'] as string,
		due_date: row['due_date'] as string,
		payment_terms: row['payment_terms'] as Invoice['payment_terms'],
		subtotal: row['subtotal'] as number,
		tax_rate: row['tax_rate'] as number,
		tax_rate_id: (row['tax_rate_id'] as number | null | undefined) ?? null,
		tax_amount: row['tax_amount'] as number,
		total: row['total'] as number,
		notes: (row['notes'] as string | null | undefined) ?? '',
		status: row['status'] as Invoice['status'],
		currency_code: (row['currency_code'] as string | null | undefined) ?? 'USD',
		business_snapshot: (row['business_snapshot'] as string | null | undefined) ?? '{}',
		client_snapshot: (row['client_snapshot'] as string | null | undefined) ?? '{}',
		payer_snapshot: (row['payer_snapshot'] as string | null | undefined) ?? '{}',
		created_at: toISOString(row['created_at'] as string | null),
		updated_at: toISOString(row['updated_at'] as string | null)
	};
}

function mapRowToEstimate(row: Record<string, unknown>): Estimate {
	const clientName = row['client_name'] as string | null | undefined;
	return {
		id: row['id'] as number,
		uuid: row['uuid'] as string,
		estimate_number: row['estimate_number'] as string,
		client_id: row['client_id'] as number,
		...(clientName !== null && clientName !== undefined ? { client_name: clientName } : {}),
		date: row['date'] as string,
		valid_until: row['valid_until'] as string,
		subtotal: row['subtotal'] as number,
		tax_rate: row['tax_rate'] as number,
		tax_rate_id: (row['tax_rate_id'] as number | null | undefined) ?? null,
		tax_amount: row['tax_amount'] as number,
		total: row['total'] as number,
		notes: (row['notes'] as string | null | undefined) ?? '',
		status: row['status'] as Estimate['status'],
		currency_code: (row['currency_code'] as string | null | undefined) ?? 'USD',
		converted_invoice_id: (row['converted_invoice_id'] as number | null | undefined) ?? null,
		business_snapshot: (row['business_snapshot'] as string | null | undefined) ?? '{}',
		client_snapshot: (row['client_snapshot'] as string | null | undefined) ?? '{}',
		payer_snapshot: (row['payer_snapshot'] as string | null | undefined) ?? '{}',
		created_at: toISOString(row['created_at'] as string | null),
		updated_at: toISOString(row['updated_at'] as string | null)
	};
}

type Db = ReturnType<typeof getDb>;

async function fetchInvoiceTotals(db: Db, defaultCurrency: string) {
	const revenueResult = await db
		.select({ total: sql<number>`COALESCE(SUM(${invoices.total}), 0)` })
		.from(invoices)
		.where(
			sql`${invoices.status} = 'paid' AND COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}`
		);
	const outstandingResult = await db
		.select({ total: sql<number>`COALESCE(SUM(${invoices.total}), 0)` })
		.from(invoices)
		.where(
			sql`${invoices.status} IN ('sent', 'overdue') AND COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}`
		);
	const overdueResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(invoices)
		.where(eq(invoices.status, 'overdue'));
	return {
		total_revenue: revenueResult[0]?.total ?? 0,
		outstanding_amount: outstandingResult[0]?.total ?? 0,
		overdue_count: overdueResult[0]?.count ?? 0
	};
}

async function fetchCounts(db: Db, defaultCurrency: string) {
	const clientResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(clients);
	const invoiceResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(invoices);
	const excludedResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(invoices)
		.where(sql`COALESCE(${invoices.currency_code}, 'USD') != ${defaultCurrency}`);
	return {
		total_clients: clientResult[0]?.count ?? 0,
		total_invoices: invoiceResult[0]?.count ?? 0,
		excluded_currency_count: excludedResult[0]?.count ?? 0
	};
}

async function fetchRecentInvoices(db: Db) {
	const rows = await db
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
	return rows.map((r) => mapRowToInvoice(r as unknown as Record<string, unknown>));
}

async function fetchEstimateStats(db: Db) {
	const estimateResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(estimates);
	const pendingResult = await db
		.select({ count: sql<number>`COUNT(*)` })
		.from(estimates)
		.where(inArray(estimates.status, ['draft', 'sent']));
	const rows = await db
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
	return {
		total_estimates: estimateResult[0]?.count ?? 0,
		pending_estimates: pendingResult[0]?.count ?? 0,
		recent_estimates: rows.map((r) => mapRowToEstimate(r as unknown as Record<string, unknown>))
	};
}

export async function getDashboardStats(): Promise<DashboardStats> {
	const db = getDb();
	const profile = await getBusinessProfile();
	const defaultCurrency = profile?.default_currency ?? 'USD';
	const totals = await fetchInvoiceTotals(db, defaultCurrency);
	const counts = await fetchCounts(db, defaultCurrency);
	const recent_invoices = await fetchRecentInvoices(db);
	const estimateStats = await fetchEstimateStats(db);
	return {
		...totals,
		...counts,
		recent_invoices,
		...estimateStats
	};
}

export async function getMonthlyRevenue(): Promise<MonthlyRevenue[]> {
	const db = getDb();
	const profile = await getBusinessProfile();
	const defaultCurrency = profile?.default_currency ?? 'USD';

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
