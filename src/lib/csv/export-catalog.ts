import { query } from '$lib/db/connection.svelte.js';
import Papa from 'papaparse';
import { downloadCsv } from './download.js';
import { getRateTiers } from '$lib/db/queries/rate-tiers.js';
import { getCatalogItemWithRates } from '$lib/db/queries/catalog.js';
import type { CatalogItem } from '$lib/types/index.js';

export async function exportCatalog(): Promise<void> {
	const items = query<CatalogItem>(
		'SELECT * FROM catalog_items ORDER BY name'
	);
	const tiers = getRateTiers();

	const rows = items.map((item) => {
		const withRates = getCatalogItemWithRates(item.id);
		const row: Record<string, unknown> = {
			uuid: item.uuid,
			name: item.name,
			rate: item.rate,
			unit: item.unit,
			category: item.category,
			sku: item.sku
		};

		for (const tier of tiers) {
			const tierRate = withRates?.rates[tier.id];
			row[`Rate: ${tier.name}`] = tierRate ?? '';
		}

		row['metadata'] = item.metadata ?? '';

		return row;
	});

	const csv = Papa.unparse(rows);
	const date = new Date().toISOString().slice(0, 10);
	await downloadCsv(csv, `catalog-${date}.csv`);
}
