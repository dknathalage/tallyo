import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const item = await repositories.catalog.getCatalogItem(id);
	if (!item) error(404, 'Catalog item not found');
	return {
		item,
		itemWithRates: await repositories.catalog.getCatalogItemWithRates(id),
		rateTiers: await repositories.rateTiers.getRateTiers(),
		auditHistory: await repositories.audit.getEntityHistory('catalog', id)
	};
};
