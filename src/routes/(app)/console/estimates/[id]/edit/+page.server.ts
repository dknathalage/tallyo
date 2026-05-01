import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const estimate = await repositories.estimates.getEstimate(id);
	if (!estimate) error(404, 'Estimate not found');
	return {
		estimate,
		lineItems: await repositories.estimates.getEstimateLineItems(id),
		clients: (await repositories.clients.getClients(undefined, { limit: 200 })).data,
		payers: await repositories.payers.getPayers(),
		catalog: (await repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 })).data,
		rateTiers: await repositories.rateTiers.getRateTiers(),
		taxRates: await repositories.taxRates.getTaxRates(),
		businessProfile: await repositories.businessProfile.getBusinessProfile()
	};
};
