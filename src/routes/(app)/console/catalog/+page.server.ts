import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = () => {
	return {
		catalog: repositories.catalog.getCatalogItems(),
		categories: repositories.catalog.getCatalogCategories(),
		rateTiers: repositories.rateTiers.getRateTiers()
	};
};
