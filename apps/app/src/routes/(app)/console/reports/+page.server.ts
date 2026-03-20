import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async () => {
	return {
		agingBuckets: await repositories.invoices.getAgingReport(),
		defaultCurrency: (await repositories.businessProfile.getBusinessProfile())?.default_currency || 'USD'
	};
};
