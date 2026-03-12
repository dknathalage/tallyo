import { execute, query, save, runRaw } from '../connection.svelte.js';
import { logAudit, computeChanges } from '../audit.js';
import type { Invoice, LineItem } from '../../types/index.js';

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

export async function createInvoice(
	data: {
		invoice_number: string;
		client_id: number;
		date: string;
		due_date: string;
		payment_terms?: string;
		subtotal: number;
		tax_rate: number;
		tax_amount: number;
		total: number;
		notes?: string;
		status?: string;
		currency_code?: string;
		business_snapshot?: string;
		client_snapshot?: string;
		payer_snapshot?: string;
	},
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number; notes?: string }>
): Promise<number> {
	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, payment_terms, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[
				crypto.randomUUID(),
				data.invoice_number,
				data.client_id,
				data.date,
				data.due_date,
				data.payment_terms ?? 'custom',
				data.subtotal,
				data.tax_rate,
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

		logAudit({
			entity_type: 'invoice',
			entity_id: invoiceId,
			action: 'create',
			context: data.invoice_number
		});

		runRaw('COMMIT');
		await save();
		return invoiceId;
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function updateInvoice(
	id: number,
	data: {
		invoice_number: string;
		client_id: number;
		date: string;
		due_date: string;
		payment_terms?: string;
		subtotal: number;
		tax_rate: number;
		tax_amount: number;
		total: number;
		notes?: string;
		status?: string;
		currency_code?: string;
		business_snapshot?: string;
		client_snapshot?: string;
		payer_snapshot?: string;
	},
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number; notes?: string }>
): Promise<void> {
	const oldInvoice = getInvoice(id);
	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`UPDATE invoices SET invoice_number = ?, client_id = ?, date = ?, due_date = ?, payment_terms = ?, subtotal = ?, tax_rate = ?, tax_amount = ?, total = ?, notes = ?, status = ?, currency_code = ?, business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = datetime('now') WHERE id = ?`,
			[
				data.invoice_number,
				data.client_id,
				data.date,
				data.due_date,
				data.payment_terms ?? 'custom',
				data.subtotal,
				data.tax_rate,
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

		if (oldInvoice) {
			const changes = computeChanges(
				oldInvoice as unknown as Record<string, unknown>,
				{ ...data, notes: data.notes ?? '', status: data.status ?? 'draft', currency_code: data.currency_code ?? 'USD' },
				['invoice_number', 'client_id', 'date', 'due_date', 'subtotal', 'tax_rate', 'total', 'notes', 'status', 'currency_code']
			);
			if (Object.keys(changes).length > 0) {
				logAudit({ entity_type: 'invoice', entity_id: id, action: 'update', changes, context: data.invoice_number });
			}
		}

		runRaw('COMMIT');
		await save();
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function deleteInvoice(id: number): Promise<void> {
	const invoice = getInvoice(id);
	runRaw('BEGIN TRANSACTION');
	try {
		execute(`DELETE FROM invoices WHERE id = ?`, [id]);
		logAudit({ entity_type: 'invoice', entity_id: id, action: 'delete', context: invoice?.invoice_number ?? '' });
		runRaw('COMMIT');
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
	await save();
}

export async function updateInvoiceStatus(id: number, status: string): Promise<void> {
	const oldInvoice = getInvoice(id);
	execute(
		`UPDATE invoices SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		[status, id]
	);
	logAudit({
		entity_type: 'invoice',
		entity_id: id,
		action: 'status_change',
		changes: { status: { old: oldInvoice?.status ?? '', new: status } },
		context: oldInvoice?.invoice_number ?? ''
	});
	await save();
}

export async function bulkDeleteInvoices(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const batch_id = crypto.randomUUID();
	const invoices = ids.map((id) => getInvoice(id));
	const placeholders = ids.map(() => '?').join(',');
	runRaw('BEGIN TRANSACTION');
	try {
		execute(`DELETE FROM line_items WHERE invoice_id IN (${placeholders})`, ids);
		execute(`DELETE FROM invoices WHERE id IN (${placeholders})`, ids);
		for (let i = 0; i < ids.length; i++) {
			logAudit({ entity_type: 'invoice', entity_id: ids[i], action: 'delete', context: invoices[i]?.invoice_number ?? '', batch_id });
		}
		runRaw('COMMIT');
		await save();
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void> {
	if (ids.length === 0) return;
	const batch_id = crypto.randomUUID();
	const invoices = ids.map((id) => getInvoice(id));
	const placeholders = ids.map(() => '?').join(',');
	execute(
		`UPDATE invoices SET status = ?, updated_at = datetime('now') WHERE id IN (${placeholders})`,
		[status, ...ids]
	);
	for (let i = 0; i < ids.length; i++) {
		logAudit({
			entity_type: 'invoice',
			entity_id: ids[i],
			action: 'status_change',
			changes: { status: { old: invoices[i]?.status ?? '', new: status } },
			context: invoices[i]?.invoice_number ?? '',
			batch_id
		});
	}
	await save();
}

export function getClientInvoices(clientId: number): Invoice[] {
	return query<Invoice>(
		`SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.client_id = ? ORDER BY i.created_at DESC`,
		[clientId]
	);
}

/**
 * Marks any 'Sent' invoice whose due_date is before today as 'Overdue'.
 * Returns the number of invoices updated.
 */
export async function markOverdueInvoices(): Promise<number> {
	const overdueIds = query<{ id: number }>(
		`SELECT id FROM invoices WHERE status = 'sent' AND due_date < date('now')`
	);
	if (overdueIds.length === 0) return 0;

	const placeholders = overdueIds.map(() => '?').join(',');
	const ids = overdueIds.map((r) => r.id);

	execute(
		`UPDATE invoices SET status = 'overdue', updated_at = datetime('now') WHERE id IN (${placeholders})`,
		ids
	);

	for (const { id } of overdueIds) {
		const inv = getInvoice(id);
		logAudit({
			entity_type: 'invoice',
			entity_id: id,
			action: 'status_change',
			changes: { status: { old: 'sent', new: 'overdue' } },
			context: inv?.invoice_number ?? ''
		});
	}

	await save();
	return overdueIds.length;
}

export async function duplicateInvoice(id: number): Promise<number> {
	const original = getInvoice(id);
	if (!original) throw new Error(`Invoice ${id} not found`);

	const { generateInvoiceNumber } = await import('../../utils/invoice-number.js');
	const newNumber = generateInvoiceNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	const originalLineItems = getInvoiceLineItems(id);

	runRaw('BEGIN TRANSACTION');
	try {
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

		logAudit({ entity_type: 'invoice', entity_id: newId, action: 'create', context: `${newNumber} (duplicated from ${original.invoice_number})` });
		runRaw('COMMIT');
		await save();
		return newId;
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}
