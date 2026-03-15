import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = () => {
	return json({
		agingBuckets: repositories.invoices.getAgingReport(),
		defaultCurrency: repositories.businessProfile.getBusinessProfile()?.default_currency || 'USD'
	});
};
