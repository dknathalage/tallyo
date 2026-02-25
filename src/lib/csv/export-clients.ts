import { query } from '$lib/db/connection.svelte.js';
import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export async function exportClients(): Promise<void> {
	const rows = query<Record<string, unknown>>(
		'SELECT uuid, name, email, phone, address FROM clients ORDER BY name'
	);
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	await downloadCsv(csv, `clients-${date}.csv`);
}
