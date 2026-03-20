import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async () => {
	return {
		businessProfile: await repositories.businessProfile.getBusinessProfile(),
		taxRates: await repositories.taxRates.getTaxRates(),
		columnMappings: await repositories.columnMappings.getColumnMappings()
	};
};
