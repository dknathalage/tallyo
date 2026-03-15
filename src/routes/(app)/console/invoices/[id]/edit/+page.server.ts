import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ params }) => {
	const id = parseInt(params.id);
	const invoice = repositories.invoices.getInvoice(id);
	if (!invoice) throw error(404, 'Invoice not found');
	return {
		invoice,
		lineItems: repositories.invoices.getInvoiceLineItems(id),
		clients: repositories.clients.getClients(undefined, { limit: 200 }).data,
		payers: repositories.payers.getPayers(),
		catalog: repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 }).data,
		rateTiers: repositories.rateTiers.getRateTiers(),
		taxRates: repositories.taxRates.getTaxRates(),
		businessProfile: repositories.businessProfile.getBusinessProfile()
	};
};
