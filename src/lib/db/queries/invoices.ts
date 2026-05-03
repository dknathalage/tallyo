import { getDb } from '../connection.js';
import { invoices, lineItems, clients, taxRates } from '../drizzle-schema.js';
import { eq, and, or, like, desc, sql, inArray } from 'drizzle-orm';
import type { Invoice, LineItem, AgingBucket, PaginationParams, PaginatedResult } from '../../types/index.js';
import { paginate } from '../../types/index.js';
import type { CreateInvoiceInput, UpdateInvoiceInput, LineItemInput } from '../../repositories/interfaces/types.js';
import { getBusinessProfile } from './business-profile.js';
import { generateInvoiceNumber } from '../number-generators.js';

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

function mapRowToLineItem(row: Record<string, unknown>): LineItem {
	return {
		id: row['id'] as number,
		uuid: row['uuid'] as string,
		invoice_id: row['invoice_id'] as number,
		description: row['description'] as string,
		quantity: row['quantity'] as number,
		rate: row['rate'] as number,
		amount: row['amount'] as number,
		notes: (row['notes'] as string | null | undefined) ?? '',
		sort_order: (row['sort_order'] as number | null | undefined) ?? 0,
		catalog_item_id: (row['catalog_item_id'] as number | null | undefined) ?? null,
		rate_tier_id: (row['rate_tier_id'] as number | null | undefined) ?? null
	};
}

export async function getInvoices(
	search?: string,
	status?: string,
	pagination?: PaginationParams
): Promise<PaginatedResult<Invoice>> {
	const db = getDb();
	const conditions: ReturnType<typeof eq>[] = [];

	if (search) {
		const searchCondition = or(
			like(invoices.invoice_number, `%${search}%`),
			like(clients.name, `%${search}%`)
		);
		if (!searchCondition) throw new Error('Failed to build search condition');
		conditions.push(searchCondition);
	}
	if (status) {
		conditions.push(eq(invoices.status, status));
	}

	const query = db
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
		.orderBy(desc(invoices.created_at));

	const rows =
		conditions.length > 0
			? await query.where(and(...conditions))
			: await query;

	const all = rows.map((r) => mapRowToInvoice(r as unknown as Record<string, unknown>));
	return paginate(all, pagination);
}

export async function getInvoice(id: number): Promise<Invoice | null> {
	const db = getDb();
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
		.where(eq(invoices.id, id));

	const first = rows[0];
	if (!first) return null;
	return mapRowToInvoice(first);
}

export async function getInvoiceLineItems(invoiceId: number): Promise<LineItem[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(lineItems)
		.where(eq(lineItems.invoice_id, invoiceId))
		.orderBy(lineItems.sort_order);

	return rows.map((r) => mapRowToLineItem(r as unknown as Record<string, unknown>));
}

/**
 * Inserts the invoice and its line items in a transaction, returns the new invoice id.
 */
export async function createInvoice(
	data: CreateInvoiceInput,
	items: LineItemInput[]
): Promise<number> {
	const db = getDb();
	return db.transaction((tx) => {
		const inserted = tx
			.insert(invoices)
			.values({
				uuid: data.uuid ?? crypto.randomUUID(),
				invoice_number: data.invoice_number,
				client_id: data.client_id,
				date: data.date,
				due_date: data.due_date,
				payment_terms: data.payment_terms ?? 'custom',
				subtotal: data.subtotal,
				tax_rate: data.tax_rate,
				tax_rate_id: data.tax_rate_id ?? null,
				tax_amount: data.tax_amount,
				total: data.total,
				notes: data.notes ?? '',
				status: data.status ?? 'draft',
				currency_code: data.currency_code ?? 'USD',
				business_snapshot: data.business_snapshot ?? '{}',
				client_snapshot: data.client_snapshot ?? '{}',
				payer_snapshot: data.payer_snapshot ?? '{}'
			})
			.returning({ id: invoices.id })
			.all()[0];

		if (!inserted) throw new Error('Failed to insert invoice');
		const invoiceId = inserted.id;

		for (const item of items) {
			tx.insert(lineItems)
				.values({
					uuid: crypto.randomUUID(),
					invoice_id: invoiceId,
					description: item.description,
					quantity: item.quantity,
					rate: item.rate,
					amount: item.amount,
					notes: item.notes ?? '',
					sort_order: item.sort_order
				})
				.run();
		}

		return invoiceId;
	});
}

/**
 * Updates the invoice and replaces its line items.
 * If tax_rate_id is provided, looks up the actual rate from tax_rates.
 */
export async function updateInvoice(
	id: number,
	data: UpdateInvoiceInput,
	items: LineItemInput[]
): Promise<void> {
	const db = getDb();

	let resolvedTaxRate = data.tax_rate;
	if (data.tax_rate_id) {
		const taxRateRows = await db
			.select({ rate: taxRates.rate })
			.from(taxRates)
			.where(eq(taxRates.id, data.tax_rate_id));
		const firstTaxRate = taxRateRows[0];
		if (firstTaxRate) {
			resolvedTaxRate = firstTaxRate.rate;
		}
	}

	await db
		.update(invoices)
		.set({
			invoice_number: data.invoice_number,
			client_id: data.client_id,
			date: data.date,
			due_date: data.due_date,
			payment_terms: data.payment_terms ?? 'custom',
			subtotal: data.subtotal,
			tax_rate: resolvedTaxRate,
			tax_rate_id: data.tax_rate_id ?? null,
			tax_amount: data.tax_amount,
			total: data.total,
			notes: data.notes ?? '',
			status: data.status ?? 'draft',
			currency_code: data.currency_code ?? 'USD',
			business_snapshot: data.business_snapshot ?? '{}',
			client_snapshot: data.client_snapshot ?? '{}',
			payer_snapshot: data.payer_snapshot ?? '{}',
			updated_at: new Date().toISOString()
		})
		.where(eq(invoices.id, id));

	await db.delete(lineItems).where(eq(lineItems.invoice_id, id));

	for (const item of items) {
		await db.insert(lineItems).values({
			uuid: crypto.randomUUID(),
			invoice_id: id,
			description: item.description,
			quantity: item.quantity,
			rate: item.rate,
			amount: item.amount,
			notes: item.notes ?? '',
			sort_order: item.sort_order
		});
	}
}

/**
 * Deletes the invoice row (cascades to line_items via FK).
 */
export async function deleteInvoice(id: number): Promise<void> {
	const db = getDb();
	await db.delete(invoices).where(eq(invoices.id, id));
}

/**
 * Updates invoice status.
 */
export async function updateInvoiceStatus(id: number, status: string): Promise<void> {
	const db = getDb();
	await db
		.update(invoices)
		.set({ status, updated_at: new Date().toISOString() })
		.where(eq(invoices.id, id));
}

/**
 * Bulk deletes invoices and their line items.
 */
export async function bulkDeleteInvoices(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db.delete(lineItems).where(inArray(lineItems.invoice_id, ids));
	await db.delete(invoices).where(inArray(invoices.id, ids));
}

/**
 * Bulk updates invoice status.
 */
export async function bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db
		.update(invoices)
		.set({ status, updated_at: new Date().toISOString() })
		.where(inArray(invoices.id, ids));
}

export async function getClientInvoices(clientId: number): Promise<Invoice[]> {
	const db = getDb();
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
		.where(eq(invoices.client_id, clientId))
		.orderBy(desc(invoices.created_at));

	return rows.map((r) => mapRowToInvoice(r as unknown as Record<string, unknown>));
}

/**
 * Marks any 'sent' invoice whose due_date is before today as 'overdue'.
 * Returns the list of invoices that were updated (for audit use).
 */
export async function markOverdueInvoices(): Promise<{ id: number; invoice_number: string }[]> {
	const db = getDb();

	const overdue = await db
		.select({ id: invoices.id, invoice_number: invoices.invoice_number })
		.from(invoices)
		.where(
			and(
				eq(invoices.status, 'sent'),
				sql`${invoices.due_date} < date('now')`
			)
		);

	if (overdue.length === 0) return [];

	const ids = overdue.map((r) => r.id);
	await db
		.update(invoices)
		.set({ status: 'overdue', updated_at: new Date().toISOString() })
		.where(inArray(invoices.id, ids));

	return overdue;
}

/**
 * Duplicates an invoice and its line items.
 * Returns the new invoice id.
 */
export async function duplicateInvoice(id: number): Promise<number> {
	const original = await getInvoice(id);
	if (!original) throw new Error(`Invoice ${id} not found`);

	const newNumber = await generateInvoiceNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	const originalItems = await getInvoiceLineItems(id);

	const db = getDb();
	return db.transaction((tx) => {
		const inserted = tx
			.insert(invoices)
			.values({
				uuid: crypto.randomUUID(),
				invoice_number: newNumber,
				client_id: original.client_id,
				date: todayStr,
				due_date: '',
				payment_terms: 'custom',
				subtotal: original.subtotal,
				tax_rate: original.tax_rate,
				tax_amount: original.tax_amount,
				total: original.total,
				notes: original.notes,
				status: 'draft',
				currency_code: original.currency_code,
				business_snapshot: original.business_snapshot,
				client_snapshot: original.client_snapshot,
				payer_snapshot: original.payer_snapshot
			})
			.returning({ id: invoices.id })
			.all()[0];

		if (!inserted) throw new Error('Failed to duplicate invoice');
		const newId = inserted.id;

		for (const item of originalItems) {
			tx.insert(lineItems)
				.values({
					uuid: crypto.randomUUID(),
					invoice_id: newId,
					description: item.description,
					quantity: item.quantity,
					rate: item.rate,
					amount: item.amount,
					notes: item.notes,
					sort_order: item.sort_order
				})
				.run();
		}

		return newId;
	});
}

function bucketForDays(
	days: number,
	buckets: { current: AgingBucket; b1to30: AgingBucket; b31to60: AgingBucket; b61to90: AgingBucket; b90plus: AgingBucket }
): AgingBucket {
	if (days <= 0) return buckets.current;
	if (days <= 30) return buckets.b1to30;
	if (days <= 60) return buckets.b31to60;
	if (days <= 90) return buckets.b61to90;
	return buckets.b90plus;
}

export async function getAgingReport(): Promise<AgingBucket[]> {
	const profile = await getBusinessProfile();
	const defaultCurrency =
		profile && profile.default_currency !== '' ? profile.default_currency : 'USD';

	const db = getDb();
	const outstanding = await db
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
			updated_at: invoices.updated_at,
			days_overdue: sql<number>`CAST(julianday('now') - julianday(${invoices.due_date}) AS INTEGER)`
		})
		.from(invoices)
		.leftJoin(clients, eq(invoices.client_id, clients.id))
		.where(
			and(
				inArray(invoices.status, ['sent', 'overdue']),
				sql`COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}`
			)
		)
		.orderBy(invoices.due_date);

	const current: AgingBucket = { label: 'Current', total: 0, invoices: [] };
	const b1to30: AgingBucket = { label: '1–30 days', total: 0, invoices: [] };
	const b31to60: AgingBucket = { label: '31–60 days', total: 0, invoices: [] };
	const b61to90: AgingBucket = { label: '61–90 days', total: 0, invoices: [] };
	const b90plus: AgingBucket = { label: '90+ days', total: 0, invoices: [] };
	const buckets: AgingBucket[] = [current, b1to30, b31to60, b61to90, b90plus];

	const bucketSet = { current, b1to30, b31to60, b61to90, b90plus };
	for (const row of outstanding) {
		const inv = mapRowToInvoice(row);
		const bucket = bucketForDays(row.days_overdue, bucketSet);
		bucket.invoices.push(inv);
		bucket.total += inv.total;
	}

	return buckets;
}
