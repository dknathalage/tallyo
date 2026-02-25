import { query } from '$lib/db/connection.svelte.js';
import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export function exportClients(): void {
	const rows = query<Record<string, unknown>>(
		'SELECT uuid, name, email, phone, address FROM clients ORDER BY name'
	);
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	downloadCsv(csv, `clients-${date}.csv`);
}
