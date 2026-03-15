import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = () => {
	return {
		estimates: repositories.estimates.getEstimates()
	};
};
