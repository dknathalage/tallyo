import { query } from '../db/connection.svelte.js';

export function generateInvoiceNumber(): string {
	const result = query<{ max_num: string | null }>(
		`SELECT MAX(invoice_number) as max_num FROM invoices`
	);

	if (result.length > 0 && result[0].max_num) {
		const current = parseInt(result[0].max_num.replace('INV-', ''), 10);
		return `INV-${String(current + 1).padStart(4, '0')}`;
	}

	return 'INV-0001';
}
