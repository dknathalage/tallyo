import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ params }) => {
	const id = parseInt(params.id);
	const item = repositories.catalog.getCatalogItem(id);
	if (!item) throw error(404, 'Catalog item not found');
	return {
		item,
		itemWithRates: repositories.catalog.getCatalogItemWithRates(id),
		rateTiers: repositories.rateTiers.getRateTiers(),
		auditHistory: repositories.audit.getEntityHistory('catalog', id)
	};
};
