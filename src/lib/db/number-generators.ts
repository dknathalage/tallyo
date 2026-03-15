/**
 * Server-side number generators that use the DB directly.
 * Only import from server-side code (+page.server.ts, +server.ts, query files).
 */
import { query } from './connection.js';

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

export function generateEstimateNumber(): string {
	const result = query<{ max_num: number | null }>(
		`SELECT MAX(CAST(SUBSTR(estimate_number, 5) AS INTEGER)) as max_num
		 FROM estimates
		 WHERE estimate_number GLOB 'EST-[0-9]*'`
	);
	const current = result.length > 0 && result[0].max_num != null ? result[0].max_num : 0;
	return `EST-${String(current + 1).padStart(4, '0')}`;
}
