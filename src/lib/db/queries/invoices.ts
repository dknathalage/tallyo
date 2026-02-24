import { execute, query, save, runRaw } from '../connection.svelte.js';
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
		subtotal: number;
		tax_rate: number;
		tax_amount: number;
		total: number;
		notes?: string;
		status?: string;
	},
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
): Promise<number> {
	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`INSERT INTO invoices (invoice_number, client_id, date, due_date, subtotal, tax_rate, tax_amount, total, notes, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[
				data.invoice_number,
				data.client_id,
				data.date,
				data.due_date,
				data.subtotal,
				data.tax_rate,
				data.tax_amount,
				data.total,
				data.notes ?? '',
				data.status ?? 'draft'
			]
		);

		const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
		const invoiceId = result[0].id;

		for (const item of lineItems) {
			execute(
				`INSERT INTO line_items (invoice_id, description, quantity, rate, amount, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
				[invoiceId, item.description, item.quantity, item.rate, item.amount, item.sort_order]
			);
		}

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
		subtotal: number;
		tax_rate: number;
		tax_amount: number;
		total: number;
		notes?: string;
		status?: string;
	},
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
): Promise<void> {
	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`UPDATE invoices SET invoice_number = ?, client_id = ?, date = ?, due_date = ?, subtotal = ?, tax_rate = ?, tax_amount = ?, total = ?, notes = ?, status = ?, updated_at = datetime('now') WHERE id = ?`,
			[
				data.invoice_number,
				data.client_id,
				data.date,
				data.due_date,
				data.subtotal,
				data.tax_rate,
				data.tax_amount,
				data.total,
				data.notes ?? '',
				data.status ?? 'draft',
				id
			]
		);

		execute(`DELETE FROM line_items WHERE invoice_id = ?`, [id]);

		for (const item of lineItems) {
			execute(
				`INSERT INTO line_items (invoice_id, description, quantity, rate, amount, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
				[id, item.description, item.quantity, item.rate, item.amount, item.sort_order]
			);
		}

		runRaw('COMMIT');
		await save();
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function deleteInvoice(id: number): Promise<void> {
	execute(`DELETE FROM invoices WHERE id = ?`, [id]);
	await save();
}

export async function updateInvoiceStatus(id: number, status: string): Promise<void> {
	execute(
		`UPDATE invoices SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		[status, id]
	);
	await save();
}

export function getClientInvoices(clientId: number): Invoice[] {
	return query<Invoice>(
		`SELECT i.*, c.name as client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.client_id = ? ORDER BY i.created_at DESC`,
		[clientId]
	);
}
