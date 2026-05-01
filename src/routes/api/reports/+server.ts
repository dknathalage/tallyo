import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const GET: RequestHandler = async () => {
	return json({
		agingBuckets: await repositories.invoices.getAgingReport(),
		defaultCurrency: (await repositories.businessProfile.getBusinessProfile())?.default_currency ?? 'USD'
	});
};
