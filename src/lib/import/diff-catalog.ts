import type { MappedRow } from './map-columns.js';

export interface DiffResult {
	newItems: MappedRow[];
	updatedItems: {
		existing: { id: number; name: string; sku: string; rate: number; unit: string; category: string };
		incoming: MappedRow;
		changes: string[];
	}[];
	unchangedCount: number;
	errorItems: MappedRow[];
	summary: { total: number; new: number; updated: number; unchanged: number; errors: number };
}

export function diffCatalog(
	mappedRows: MappedRow[],
	existingItems: {
		id: number;
		name: string;
		sku: string;
		rate: number;
		unit: string;
		category: string;
	}[]
): DiffResult {
	const existingBySku = new Map<string, (typeof existingItems)[number]>();
	for (const item of existingItems) {
		if (item.sku.trim()) {
			existingBySku.set(item.sku.trim().toLowerCase(), item);
		}
	}

	const newItems: MappedRow[] = [];
	const updatedItems: DiffResult['updatedItems'] = [];
	let unchangedCount = 0;
	const errorItems: MappedRow[] = [];

	for (const row of mappedRows) {
		if (row._errors.length > 0) {
			errorItems.push(row);
			continue;
		}

		const skuKey = row.sku.trim().toLowerCase();
		const existing = skuKey ? existingBySku.get(skuKey) : undefined;

		if (!existing) {
			newItems.push(row);
			continue;
		}

		const changes: string[] = [];
		if (existing.name !== row.name) changes.push(`Name: "${existing.name}" → "${row.name}"`);
		if (existing.rate !== row.rate)
			changes.push(`Rate: $${existing.rate.toFixed(2)} → $${row.rate.toFixed(2)}`);
		if (existing.unit !== row.unit)
			changes.push(`Unit: "${existing.unit}" → "${row.unit}"`);
		if (existing.category !== row.category)
			changes.push(`Category: "${existing.category}" → "${row.category}"`);

		if (changes.length > 0) {
			updatedItems.push({ existing, incoming: row, changes });
		} else {
			unchangedCount++;
		}
	}

	return {
		newItems,
		updatedItems,
		unchangedCount,
		errorItems,
		summary: {
			total: mappedRows.length,
			new: newItems.length,
			updated: updatedItems.length,
			unchanged: unchangedCount,
			errors: errorItems.length
		}
	};
}
