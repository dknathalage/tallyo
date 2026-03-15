import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = () => {
	return {
		rateTiers: repositories.rateTiers.getRateTiers(),
		payers: repositories.payers.getPayers()
	};
};
