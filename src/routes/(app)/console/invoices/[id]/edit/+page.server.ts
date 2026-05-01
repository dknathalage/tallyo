import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const invoice = await repositories.invoices.getInvoice(id);
	if (!invoice) error(404, 'Invoice not found');
	return {
		invoice,
		lineItems: await repositories.invoices.getInvoiceLineItems(id),
		clients: (await repositories.clients.getClients(undefined, { limit: 200 })).data,
		payers: await repositories.payers.getPayers(),
		catalog: (await repositories.catalog.getCatalogItems(undefined, undefined, { limit: 200 })).data,
		rateTiers: await repositories.rateTiers.getRateTiers(),
		taxRates: await repositories.taxRates.getTaxRates(),
		businessProfile: await repositories.businessProfile.getBusinessProfile()
	};
};
