import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { generateEstimateNumber } from '$lib/utils/estimate-number.js';

export const load: PageServerLoad = async () => {
	return {
		clients: (await repositories.clients.getClients(undefined, { limit: 200 })).data,
		payers: await repositories.payers.getPayers(),
		catalog: (await repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 })).data,
		rateTiers: await repositories.rateTiers.getRateTiers(),
		taxRates: await repositories.taxRates.getTaxRates(),
		businessProfile: await repositories.businessProfile.getBusinessProfile(),
		nextEstimateNumber: await generateEstimateNumber()
	};
};
