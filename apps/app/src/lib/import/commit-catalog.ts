import type { DiffResult } from './diff-catalog.js';

function stripRaw(diff: DiffResult): DiffResult {
	return {
		...diff,
		newItems: diff.newItems.map(({ _raw, _errors, ...rest }) => ({ ...rest, _raw: {}, _errors: [] })),
		updatedItems: diff.updatedItems.map((item) => ({
			...item,
			incoming: { ...item.incoming, _raw: {}, _errors: [] }
		})),
		errorItems: []
	};
}

export async function commitCatalogImport(
	diff: DiffResult,
	options: { updateExisting: boolean }
): Promise<{ inserted: number; updated: number; batchId: string }> {
	const res = await fetch('/api/import/catalog', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ diff: stripRaw(diff), options })
	});
	if (!res.ok) {
		const err = await res.text();
		throw new Error(`Catalog import failed: ${err}`);
	}
	return res.json();
}
