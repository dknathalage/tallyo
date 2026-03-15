/**
 * Invoice number generator using the database to find the next sequential number.
 * Only import from server-side code (+page.server.ts, +server.ts, query files).
 */
import { query } from '../db/connection.js';

/**
 * Generates the next sequential invoice number in INV-XXXX format.
 * Uses the database to find the current max and increment by 1.
 */
export function generateInvoiceNumber(): string {
	const result = query<{ max_num: number | null }>(
		`SELECT MAX(CAST(SUBSTR(invoice_number, 5) AS INTEGER)) as max_num
		 FROM invoices
		 WHERE invoice_number GLOB 'INV-[0-9]*'`
	);
	const maxNum = result.length > 0 ? result[0].max_num : null;
	const next = maxNum !== null && maxNum > 0 ? maxNum + 1 : 1;
	return `INV-${String(next).padStart(4, '0')}`;
}
