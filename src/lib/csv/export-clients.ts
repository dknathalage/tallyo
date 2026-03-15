import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export async function exportClients(): Promise<void> {
	const res = await fetch('/api/clients');
	const clients = await res.json() as Array<Record<string, unknown>>;
	const rows = clients.map((c) => ({
		uuid: c.uuid,
		name: c.name,
		email: c.email,
		phone: c.phone,
		address: c.address
	}));
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	downloadCsv(csv, `clients-${date}.csv`);
}
