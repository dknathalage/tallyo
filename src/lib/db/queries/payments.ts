import { getDb } from '../connection.js';
import { payments } from '../drizzle-schema.js';
import { eq, desc, sql } from 'drizzle-orm';
import type { Payment } from '../../types/index.js';

function mapRow(row: Record<string, unknown>): Payment {
	return {
		id: row['id'] as number,
		uuid: row['uuid'] as string,
		invoice_id: row['invoice_id'] as number,
		amount: row['amount'] as number,
		payment_date: row['payment_date'] as string,
		method: (row['method'] as string) ?? '',
		notes: (row['notes'] as string) ?? '',
		created_at: (row['created_at'] as string) ?? '',
		updated_at: (row['updated_at'] as string) ?? ''
	};
}

export async function getInvoicePayments(invoiceId: number): Promise<Payment[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(payments)
		.where(eq(payments.invoice_id, invoiceId))
		.orderBy(desc(payments.payment_date), desc(payments.created_at));
	return rows.map((r) => mapRow(r as Record<string, unknown>));
}

export async function getInvoiceTotalPaid(invoiceId: number): Promise<number> {
	const db = getDb();
	const result = await db
		.select({
			total: sql<number>`COALESCE(SUM(${payments.amount}), 0)`
		})
		.from(payments)
		.where(eq(payments.invoice_id, invoiceId));
	return result[0]?.total ?? 0;
}

export async function createPayment(data: {
	invoice_id: number;
	amount: number;
	payment_date: string;
	method?: string;
	notes?: string;
}): Promise<number> {
	const db = getDb();

	const result = await db
		.insert(payments)
		.values({
			uuid: crypto.randomUUID(),
			invoice_id: data.invoice_id,
			amount: data.amount,
			payment_date: data.payment_date,
			method: data.method ?? '',
			notes: data.notes ?? ''
		})
		.returning({ id: payments.id });

	const inserted = result[0];
	if (!inserted) throw new Error('Failed to insert payment');
	return inserted.id;
}

export async function deletePayment(id: number): Promise<void> {
	const db = getDb();
	await db.delete(payments).where(eq(payments.id, id));
}
