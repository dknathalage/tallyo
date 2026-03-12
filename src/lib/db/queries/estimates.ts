import { execute, query } from '../connection.svelte.js';
import { generateInvoiceNumber } from '../../utils/invoice-number.js';
import type { Estimate, EstimateLineItem } from '../../types/index.js';
import type { CreateEstimateInput, UpdateEstimateInput, LineItemInput } from '../../repositories/interfaces/types.js';

export function getEstimates(search?: string, status?: string): Estimate[] {
	let sql = `SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id`;
	const params: unknown[] = [];
	const conditions: string[] = [];

	if (search) {
		conditions.push(`(e.estimate_number LIKE ? OR c.name LIKE ?)`);
		params.push(`%${search}%`, `%${search}%`);
	}
	if (status) {
		conditions.push(`e.status = ?`);
		params.push(status);
	}

	if (conditions.length > 0) {
		sql += ` WHERE ` + conditions.join(' AND ');
	}

	sql += ` ORDER BY e.created_at DESC`;
	return query<Estimate>(sql, params);
}

export function getEstimate(id: number): Estimate | null {
	const results = query<Estimate>(
		`SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.id = ?`,
		[id]
	);
	return results.length > 0 ? results[0] : null;
}

export function getEstimateLineItems(estimateId: number): EstimateLineItem[] {
	return query<EstimateLineItem>(
		`SELECT * FROM estimate_line_items WHERE estimate_id = ? ORDER BY sort_order`,
		[estimateId]
	);
}

/**
 * Pure SQL: inserts the estimate and its line items, returns the new estimate id.
 * No transaction management, no audit logging, no save().
 */
export async function createEstimate(
	data: CreateEstimateInput,
	lineItems: LineItemInput[]
): Promise<number> {
	execute(
		`INSERT INTO estimates (uuid, estimate_number, client_id, date, valid_until, subtotal, tax_rate, tax_rate_id, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			data.uuid ?? crypto.randomUUID(),
			data.estimate_number,
			data.client_id,
			data.date,
			data.valid_until,
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
	const estimateId = result[0].id;

	for (const item of lineItems) {
		execute(
			`INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			[crypto.randomUUID(), estimateId, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
		);
	}

	return estimateId;
}

/**
 * Pure SQL: updates the estimate and replaces its line items.
 * No transaction management, no audit logging, no save().
 */
export async function updateEstimate(
	id: number,
	data: UpdateEstimateInput,
	lineItems: LineItemInput[]
): Promise<void> {
	execute(
		`UPDATE estimates SET estimate_number = ?, client_id = ?, date = ?, valid_until = ?, subtotal = ?, tax_rate = ?, tax_rate_id = ?, tax_amount = ?, total = ?, notes = ?, status = ?, currency_code = ?, business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = datetime('now') WHERE id = ?`,
		[
			data.estimate_number,
			data.client_id,
			data.date,
			data.valid_until,
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
			data.payer_snapshot ?? '{}',
			id
		]
	);

	execute(`DELETE FROM estimate_line_items WHERE estimate_id = ?`, [id]);

	for (const item of lineItems) {
		execute(
			`INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			[crypto.randomUUID(), id, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
		);
	}
}

/**
 * Pure SQL: deletes the estimate.
 * No transaction management, no audit logging, no save().
 */
export async function deleteEstimate(id: number): Promise<void> {
	execute(`DELETE FROM estimates WHERE id = ?`, [id]);
}

/**
 * Pure SQL: updates estimate status.
 * No audit logging, no save().
 */
export async function updateEstimateStatus(id: number, status: string): Promise<void> {
	execute(
		`UPDATE estimates SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		[status, id]
	);
}

/**
 * Pure SQL: bulk deletes estimates and their line items.
 * No transaction management, no audit logging, no save().
 */
export async function bulkDeleteEstimates(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM estimate_line_items WHERE estimate_id IN (${placeholders})`, ids);
	execute(`DELETE FROM estimates WHERE id IN (${placeholders})`, ids);
}

/**
 * Pure SQL: bulk updates estimate status.
 * No audit logging, no save().
 */
export async function bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void> {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(
		`UPDATE estimates SET status = ?, updated_at = datetime('now') WHERE id IN (${placeholders})`,
		[status, ...ids]
	);
}

export function getClientEstimates(clientId: number): Estimate[] {
	return query<Estimate>(
		`SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.client_id = ? ORDER BY e.created_at DESC`,
		[clientId]
	);
}

/**
 * Pure SQL: converts an accepted estimate into an invoice.
 * No transaction management, no audit logging, no save().
 * Returns an object with both the new invoice id and info needed for audit.
 */
export async function convertEstimateToInvoice(estimateId: number): Promise<{ invoiceId: number; invoiceNumber: string; estimateNumber: string }> {
	const estimate = getEstimate(estimateId);
	if (!estimate) throw new Error('Estimate not found');
	if (estimate.status !== 'accepted') throw new Error('Only accepted estimates can be converted to invoices');
	if (estimate.converted_invoice_id !== null) throw new Error('Estimate has already been converted to an invoice');

	const lineItems = getEstimateLineItems(estimateId);
	const invoiceNumber = generateInvoiceNumber();

	execute(
		`INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			invoiceNumber,
			estimate.client_id,
			estimate.date,
			estimate.valid_until,
			estimate.subtotal,
			estimate.tax_rate,
			estimate.tax_amount,
			estimate.total,
			estimate.notes,
			'draft',
			estimate.currency_code,
			estimate.business_snapshot,
			estimate.client_snapshot,
			estimate.payer_snapshot
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

	execute(
		`UPDATE estimates SET converted_invoice_id = ?, updated_at = datetime('now') WHERE id = ?`,
		[invoiceId, estimateId]
	);

	return { invoiceId, invoiceNumber, estimateNumber: estimate.estimate_number };
}

/**
 * Pure SQL: duplicates an estimate and its line items.
 * No transaction management, no audit logging, no save().
 * Returns the new estimate id and its number.
 */
export async function duplicateEstimate(id: number): Promise<{ newId: number; newNumber: string; originalNumber: string }> {
	const original = getEstimate(id);
	if (!original) throw new Error(`Estimate ${id} not found`);

	const { generateEstimateNumber } = await import('../../utils/estimate-number.js');
	const newNumber = generateEstimateNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	const originalLineItems = getEstimateLineItems(id);

	execute(
		`INSERT INTO estimates (uuid, estimate_number, client_id, date, valid_until, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			newNumber,
			original.client_id,
			todayStr,
			'',
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
			`INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			[crypto.randomUUID(), newId, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
		);
	}

	return { newId, newNumber, originalNumber: original.estimate_number };
}
