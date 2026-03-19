import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/postgres/index.js';

export const GET: RequestHandler = async () => {
	const items = (await repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 })).data;
	const tiers = await repositories.rateTiers.getRateTiers();

	const rows = [];
	for (const item of items) {
		const withRates = await repositories.catalog.getCatalogItemWithRates(item.id);
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
		rows.push(row);
	}

	return json({ rows, tiers });
};
