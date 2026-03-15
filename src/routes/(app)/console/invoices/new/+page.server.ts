import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { generateInvoiceNumber } from '$lib/utils/invoice-number.js';

export const load: PageServerLoad = () => {
	return {
		clients: repositories.clients.getClients(undefined, { limit: 200 }).data,
		payers: repositories.payers.getPayers(),
		catalog: repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 }).data,
		rateTiers: repositories.rateTiers.getRateTiers(),
		taxRates: repositories.taxRates.getTaxRates(),
		businessProfile: repositories.businessProfile.getBusinessProfile(),
		nextInvoiceNumber: generateInvoiceNumber()
	};
};
