import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ params }) => {
	const id = parseInt(params.id);
	const payer = repositories.payers.getPayer(id);
	if (!payer) throw error(404, 'Payer not found');
	return {
		payer,
		auditHistory: repositories.audit.getEntityHistory('payer', id),
		linkedClients: repositories.payers.getPayerClients(id)
	};
};
