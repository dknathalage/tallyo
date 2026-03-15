import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ params }) => {
	const id = parseInt(params.id);
	const estimate = repositories.estimates.getEstimate(id);
	if (!estimate) throw error(404, 'Estimate not found');
	return {
		estimate,
		lineItems: repositories.estimates.getEstimateLineItems(id),
		clients: repositories.clients.getClients(),
		payers: repositories.payers.getPayers(),
		catalog: repositories.catalog.getCatalogItems(),
		rateTiers: repositories.rateTiers.getRateTiers(),
		taxRates: repositories.taxRates.getTaxRates(),
		businessProfile: repositories.businessProfile.getBusinessProfile()
	};
};
