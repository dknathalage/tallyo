import { query } from '../db/connection.svelte.js';

export function generateInvoiceNumber(): string {
	// Use numeric MAX on the integer suffix of invoice numbers matching 'INV-<digits>'.
	// The GLOB filter excludes non-standard formats (e.g. 'INV-CUSTOM') so they never
	// produce NaN.  CAST converts the suffix to an integer, giving true numeric ordering
	// instead of the lexicographic ordering that MAX() on strings would produce.
	const result = query<{ max_num: number | null }>(
		`SELECT MAX(CAST(SUBSTR(invoice_number, 5) AS INTEGER)) as max_num
		 FROM invoices
		 WHERE invoice_number GLOB 'INV-[0-9]*'`
	);

	const maxNum = result.length > 0 ? result[0].max_num : null;
	// Default to 1 when DB is empty or contains no standard-format invoice numbers.
	const next = maxNum !== null && maxNum > 0 ? maxNum + 1 : 1;
	return `INV-${String(next).padStart(4, '0')}`;
}
