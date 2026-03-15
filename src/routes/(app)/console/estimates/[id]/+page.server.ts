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
		auditHistory: repositories.audit.getEntityHistory('estimate', id)
	};
};
