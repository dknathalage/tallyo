import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { generateEstimateNumber } from '$lib/utils/estimate-number.js';

export const load: PageServerLoad = () => {
	return {
		clients: repositories.clients.getClients(),
		payers: repositories.payers.getPayers(),
		catalog: repositories.catalog.getCatalogItems(),
		rateTiers: repositories.rateTiers.getRateTiers(),
		taxRates: repositories.taxRates.getTaxRates(),
		businessProfile: repositories.businessProfile.getBusinessProfile(),
		nextEstimateNumber: generateEstimateNumber()
	};
};
