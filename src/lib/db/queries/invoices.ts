import { execute, query, runRaw } from '../connection.svelte.js';
import type { Invoice, LineItem, AgingBucket } from '../../types/index.js';
import type { CreateInvoiceInput, UpdateInvoiceInput, LineItemInput } from '../../repositories/interfaces/types.js';
import { getBusinessProfile } from './business-profile.js';

export function getInvoices(search?: string, status?: string): Invoice[] {
	let sql = `SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id`;
	const params: unknown[] = [];
	const conditions: string[] = [];

	if (search) {
		conditions.push(`(i.invoice_number LIKE ? OR c.name LIKE ?)`);
		params.push(`%${search}%`, `%${search}%`);
	}
	if (status) {
		conditions.push(`i.status = ?`);
		params.push(status);
	}

	if (conditions.length > 0) {
		sql += ` WHERE ` + conditions.join(' AND ');
	}

	sql += ` ORDER BY i.created_at DESC`;
	return query<Invoice>(sql, params);
}

export function getInvoice(id: number): Invoice | null {
	const results = query<Invoice>(
		`SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?`,
		[id]
	);
	return results.length > 0 ? results[0] : null;
}

export function getInvoiceLineItems(invoiceId: number): LineItem[] {
	return query<LineItem>(
		`SELECT * FROM line_items WHERE invoice_id = ? ORDER BY sort_order`,
		[invoiceId]
	);
}

/**
 * Pure SQL: inserts the invoice and its line items, returns the new invoice id.
 * No transaction management, no audit logging, no save().
 */
export async function createInvoice(
	data: CreateInvoiceInput,
	lineItems: LineItemInput[]
): Promise<number> {
	execute(
		`INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, payment_terms, subtotal, tax_rate, tax_rate_id, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			data.invoice_number,
			data.client_id,
			data.date,
			data.due_date,
			data.payment_terms ?? 'custom',
			data.subtotal,
			data.tax_rate,
			data.tax_rate_id ?? null,
			data.tax_amount,
			data.total,
			data.notes ?? '',
			data.status ?? 'draft',
			data.currency_code ?? 'USD',
			data.business_snapshot ?? '{}',
			data.client_snapshot ?? '{}',
			data.payer_snapshot ?? '{}'
		]
	);

	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	const invoiceId = result[0].id;

	for (const item of lineItems) {
		execute(
			`INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			[crypto.randomUUID(), invoiceId, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
		);
	}

	return invoiceId;
}

/**
 * Pure SQL: updates the invoice and replaces its line items.
 * No transaction management, no audit logging, no save().
 * If tax_rate_id is provided, looks up the actual rate from tax_rates.
 */
export async function updateInvoice(
	id: number,
	data: UpdateInvoiceInput,
	lineItems: LineItemInput[]
): Promise<void> {
	// If a tax_rate_id is provided, look up the actual rate from the tax_rates table
	let resolvedTaxRate = data.tax_rate;
	if (data.tax_rate_id) {
		const taxRateRow = query<{ rate: number }>(`SELECT rate FROM tax_rates WHERE id = ?`, [data.tax_rate_id]);
		if (taxRateRow.length > 0) {
			resolvedTaxRate = taxRateRow[0].rate;
		}
	}

	execute(
		`UPDATE invoices SET invoice_number = ?, client_id = ?, date = ?, due_date = ?, payment_terms = ?, subtotal = ?, tax_rate = ?, tax_rate_id = ?, tax_amount = ?, total = ?, notes = ?, status = ?, currency_code = ?, business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = datetime('now') WHERE id = ?`,
		[
			data.invoice_number,
			data.client_id,
			data.date,
			data.due_date,
			data.payment_terms ?? 'custom',
			data.subtotal,
			resolvedTaxRate,
			data.tax_rate_id ?? null,
			data.tax_amount,
			data.total,
			data.notes ?? '',
			data.status ?? 'draft',
			data.currency_code ?? 'USD',
			data.business_snapshot ?? '{}',
			data.client_snapshot ?? '{}',
			data.payer_snapshot ?? '{}',
			id
		]
	);

	execute(`DELETE FROM line_items WHERE invoice_id = ?`, [id]);

	for (const item of lineItems) {
		execute(
			`INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			[crypto.randomUUID(), id, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
		);
	}
}

/**
 * Pure SQL: deletes the invoice row (cascades to line_items via FK or handled separately).
 * No transaction management, no audit logging, no save().
 */
export async function deleteInvoice(id: number): Promise<void> {
	execute(`DELETE FROM invoices WHERE id = ?`, [id]);
}

/**
 * Pure SQL: updates invoice status.
 * No audit logging, no save().
 */
export async function updateInvoiceStatus(id: number, status: string): Promise<void> {
	execute(
		`UPDATE invoices SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		[status, id]
	);
}

/**
 * Pure SQL: bulk deletes invoices and their line items.
 * No transaction management, no audit logging, no save().
 */
export async function bulkDeleteInvoices(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM line_items WHERE invoice_id IN (${placeholders})`, ids);
	execute(`DELETE FROM invoices WHERE id IN (${placeholders})`, ids);
}

/**
 * Pure SQL: bulk updates invoice status.
 * No audit logging, no save().
 */
export async function bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void> {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(
		`UPDATE invoices SET status = ?, updated_at = datetime('now') WHERE id IN (${placeholders})`,
		[status, ...ids]
	);
}

export function getClientInvoices(clientId: number): Invoice[] {
	return query<Invoice>(
		`SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.client_id = ? ORDER BY i.created_at DESC`,
		[clientId]
	);
}

/**
 * Marks any 'sent' invoice whose due_date is before today as 'overdue'.
 * Pure SQL: returns the list of invoices that were updated (for audit use).
 * No audit logging, no save().
 */
export async function markOverdueInvoices(): Promise<Array<{ id: number; invoice_number: string }>> {
	const overdue = query<{ id: number; invoice_number: string }>(
		`SELECT id, invoice_number FROM invoices WHERE status = 'sent' AND due_date < date('now')`
	);
	if (overdue.length === 0) return [];

	const placeholders = overdue.map(() => '?').join(',');
	const ids = overdue.map((r) => r.id);

	execute(
		`UPDATE invoices SET status = 'overdue', updated_at = datetime('now') WHERE id IN (${placeholders})`,
		ids
	);

	return overdue;
}

/**
 * Pure SQL: duplicates an invoice and its line items.
 * No transaction management, no audit logging, no save().
 * Returns the new invoice id.
 */
export async function duplicateInvoice(id: number): Promise<number> {
	const original = getInvoice(id);
	if (!original) throw new Error(`Invoice ${id} not found`);

	const { generateInvoiceNumber } = await import('../../utils/invoice-number.js');
	const newNumber = generateInvoiceNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	const originalLineItems = getInvoiceLineItems(id);

	execute(
		`INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, payment_terms, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			newNumber,
			original.client_id,
			todayStr,
			'',
			'custom',
			original.subtotal,
			original.tax_rate,
			original.tax_amount,
			original.total,
			original.notes ?? '',
			'draft',
			original.currency_code ?? 'USD',
			original.business_snapshot ?? '{}',
			original.client_snapshot ?? '{}',
			original.payer_snapshot ?? '{}'
		]
	);

	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	const newId = result[0].id;

	for (const item of originalLineItems) {
		execute(
			`INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			[crypto.randomUUID(), newId, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
		);
	}

	return newId;
}

export function getAgingReport(): AgingBucket[] {
	const profile = getBusinessProfile();
	const defaultCurrency = profile?.default_currency || 'USD';

	// Fetch all outstanding invoices (sent + overdue) with days overdue
	const outstanding = query<Invoice & { days_overdue: number }>(
		`SELECT i.*, c.name as client_name,
		        CAST(julianday('now') - julianday(i.due_date) AS INTEGER) as days_overdue
		 FROM invoices i
		 LEFT JOIN clients c ON i.client_id = c.id
		 WHERE i.status IN ('sent', 'overdue')
		   AND COALESCE(i.currency_code, 'USD') = ?
		 ORDER BY i.due_date ASC`,
		[defaultCurrency]
	);

	const buckets: AgingBucket[] = [
		{ label: 'Current', total: 0, invoices: [] },
		{ label: '1–30 days', total: 0, invoices: [] },
		{ label: '31–60 days', total: 0, invoices: [] },
		{ label: '61–90 days', total: 0, invoices: [] },
		{ label: '90+ days', total: 0, invoices: [] }
	];

	for (const inv of outstanding) {
		const days = inv.days_overdue;
		let bucket: AgingBucket;
		if (days <= 0) {
			bucket = buckets[0]; // Current
		} else if (days <= 30) {
			bucket = buckets[1];
		} else if (days <= 60) {
			bucket = buckets[2];
		} else if (days <= 90) {
			bucket = buckets[3];
		} else {
			bucket = buckets[4];
		}
		bucket.invoices.push(inv);
		bucket.total += inv.total;
	}

	return buckets;
}
