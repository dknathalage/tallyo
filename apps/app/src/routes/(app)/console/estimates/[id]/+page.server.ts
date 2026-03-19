import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/postgres/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const estimate = await repositories.estimates.getEstimate(id);
	if (!estimate) throw error(404, 'Estimate not found');
	return {
		estimate,
		lineItems: await repositories.estimates.getEstimateLineItems(id),
		auditHistory: await repositories.audit.getEntityHistory('estimate', id)
	};
};
