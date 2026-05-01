import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const client = await repositories.clients.getClient(id);
	if (!client) error(404, 'Client not found');
	return {
		client,
		rateTiers: await repositories.rateTiers.getRateTiers(),
		payers: await repositories.payers.getPayers(),
		revenueSummary: await repositories.clients.getClientRevenueSummary(id),
		auditHistory: await repositories.audit.getEntityHistory('client', id),
		invoices: await repositories.invoices.getClientInvoices(id),
		estimates: await repositories.estimates.getClientEstimates(id),
		payer: client.payer_id ? await repositories.payers.getPayer(client.payer_id) : null
	};
};
