import type { DiffResult } from './diff-catalog.js';

export async function commitCatalogImport(
	diff: DiffResult,
	options: { updateExisting: boolean }
): Promise<{ inserted: number; updated: number; batchId: string }> {
	const res = await fetch('/api/import/catalog', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ diff, options })
	});
	if (!res.ok) {
		const err = await res.text();
		throw new Error(`Catalog import failed: ${err}`);
	}
	return res.json();
}
