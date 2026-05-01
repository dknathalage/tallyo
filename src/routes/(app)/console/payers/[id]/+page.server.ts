import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const payer = await repositories.payers.getPayer(id);
	if (!payer) error(404, 'Payer not found');
	return {
		payer,
		auditHistory: await repositories.audit.getEntityHistory('payer', id),
		linkedClients: await repositories.payers.getPayerClients(id)
	};
};
