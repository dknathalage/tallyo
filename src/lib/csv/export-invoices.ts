import { query } from '$lib/db/connection.svelte.js';
import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export function exportInvoices(): void {
	const rows = query<Record<string, unknown>>(`
		SELECT i.uuid as invoice_uuid, i.invoice_number, c.name as client_name,
		       COALESCE(c.email,'') as client_email, i.date, i.due_date, i.tax_rate,
		       COALESCE(i.notes,'') as notes, i.status,
		       COALESCE(i.currency_code, 'USD') as currency_code,
		       li.description as line_description, li.quantity as line_quantity,
		       li.rate as line_rate, li.amount as line_amount, li.sort_order as line_sort_order,
		       COALESCE(li.notes,'') as line_notes,
		       COALESCE(i.business_snapshot, '{}') as business_snapshot,
		       COALESCE(i.client_snapshot, '{}') as client_snapshot,
		       COALESCE(i.payer_snapshot, '{}') as payer_snapshot
		FROM invoices i LEFT JOIN clients c ON i.client_id = c.id
		INNER JOIN line_items li ON li.invoice_id = i.id
		ORDER BY i.invoice_number, li.sort_order
	`);
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	downloadCsv(csv, `invoices-${date}.csv`);
}
