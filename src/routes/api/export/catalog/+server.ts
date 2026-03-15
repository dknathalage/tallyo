import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = () => {
	const items = repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 }).data;
	const tiers = repositories.rateTiers.getRateTiers();

	const rows = items.map((item) => {
		const withRates = repositories.catalog.getCatalogItemWithRates(item.id);
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

	return json({ rows, tiers });
};
