import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async () => {
	return {
		rateTiers: await repositories.rateTiers.getRateTiers()
	};
};
