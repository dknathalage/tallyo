/**
 * Server-side number generators that use the DB directly.
 * Only import from server-side code (+page.server.ts, +server.ts, query files).
 */
import { getDb } from './connection.js';
import { invoices, estimates } from './drizzle-schema.js';
import { sql } from 'drizzle-orm';

export async function generateInvoiceNumber(): Promise<string> {
	const db = getDb();
	const result = await db
		.select({
			max_num: sql<number | null>`MAX(CAST(substr(${invoices.invoice_number}, 5) AS INTEGER))`
		})
		.from(invoices)
		.where(sql`${invoices.invoice_number} LIKE 'INV-%'`);
	const maxNum = result.length > 0 ? result[0].max_num : null;
	const next = maxNum !== null && maxNum > 0 ? maxNum + 1 : 1;
	return `INV-${String(next).padStart(4, '0')}`;
}

export async function generateEstimateNumber(): Promise<string> {
	const db = getDb();
	const result = await db
		.select({
			max_num: sql<number | null>`MAX(CAST(substr(${estimates.estimate_number}, 5) AS INTEGER))`
		})
		.from(estimates)
		.where(sql`${estimates.estimate_number} LIKE 'EST-%'`);
	const current = result.length > 0 && result[0].max_num != null ? result[0].max_num : 0;
	return `EST-${String(current + 1).padStart(4, '0')}`;
}
