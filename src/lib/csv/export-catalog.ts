import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export async function exportCatalog(): Promise<void> {
	const res = await fetch('/api/export/catalog');
	const { rows } = await res.json() as { rows: Record<string, unknown>[] };
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	downloadCsv(csv, `catalog-${date}.csv`);
}
