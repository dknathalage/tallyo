import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ params }) => {
	const id = parseInt(params.id);
	const client = repositories.clients.getClient(id);
	if (!client) throw error(404, 'Client not found');
	return {
		client,
		rateTiers: repositories.rateTiers.getRateTiers(),
		payers: repositories.payers.getPayers(),
		revenueSummary: repositories.clients.getClientRevenueSummary(id),
		auditHistory: repositories.audit.getEntityHistory('client', id),
		invoices: repositories.invoices.getClientInvoices(id),
		estimates: repositories.estimates.getClientEstimates(id),
		payer: client.payer_id ? repositories.payers.getPayer(client.payer_id) : null
	};
};
