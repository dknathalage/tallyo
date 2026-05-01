import Papa from 'papaparse';
import { downloadCsv } from './download.js';

export async function exportEstimates(): Promise<void> {
	const res = await fetch('/api/export/estimates');
	const rows = await res.json() as Record<string, unknown>[];
	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	downloadCsv(csv, `estimates-${date}.csv`);
}
