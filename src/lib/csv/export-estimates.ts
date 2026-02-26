import { query } from '$lib/db/connection.svelte.js';
import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export function exportEstimates(): void {
	const rows = query<Record<string, unknown>>(`
		SELECT e.uuid as estimate_uuid, e.estimate_number, c.name as client_name,
		       COALESCE(c.email,'') as client_email, e.date, e.valid_until, e.tax_rate,
		       COALESCE(e.notes,'') as notes, e.status,
		       COALESCE(e.currency_code, 'USD') as currency_code,
		       eli.description as line_description, eli.quantity as line_quantity,
		       eli.rate as line_rate, eli.amount as line_amount, eli.sort_order as line_sort_order,
		       COALESCE(eli.notes,'') as line_notes,
		       COALESCE(e.business_snapshot, '{}') as business_snapshot,
		       COALESCE(e.client_snapshot, '{}') as client_snapshot,
		       COALESCE(e.payer_snapshot, '{}') as payer_snapshot
		FROM estimates e LEFT JOIN clients c ON e.client_id = c.id
		INNER JOIN estimate_line_items eli ON eli.estimate_id = e.id
		ORDER BY e.estimate_number, eli.sort_order
	`);
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	downloadCsv(csv, `estimates-${date}.csv`);
}
