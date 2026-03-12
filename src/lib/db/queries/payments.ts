import { execute, query, save } from '../connection.svelte.js';
import type { Payment } from '../../types/index.js';

export function getInvoicePayments(invoiceId: number): Payment[] {
	return query<Payment>(
		`SELECT * FROM payments WHERE invoice_id = ? ORDER BY payment_date DESC, created_at DESC`,
		[invoiceId]
	);
}

export function getInvoiceTotalPaid(invoiceId: number): number {
	const result = query<{ total: number | null }>(
		`SELECT SUM(amount) as total FROM payments WHERE invoice_id = ?`,
		[invoiceId]
	);
	return result[0]?.total ?? 0;
}

export async function createPayment(data: {
	invoice_id: number;
	amount: number;
	payment_date: string;
	method?: string;
	notes?: string;
}): Promise<number> {
	execute(
		`INSERT INTO payments (uuid, invoice_id, amount, payment_date, method, notes) VALUES (?, ?, ?, ?, ?, ?)`,
		[crypto.randomUUID(), data.invoice_id, data.amount, data.payment_date, data.method ?? '', data.notes ?? '']
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	await save();
	return result[0].id;
}

export async function deletePayment(id: number): Promise<void> {
	execute(`DELETE FROM payments WHERE id = ?`, [id]);
	await save();
}
