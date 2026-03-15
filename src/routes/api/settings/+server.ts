import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = () => {
	return json({
		profile: repositories.businessProfile.getBusinessProfile(),
		taxRates: repositories.taxRates.getTaxRates(),
		columnMappings: repositories.columnMappings.getColumnMappings()
	});
};

export const POST: RequestHandler = async ({ request }) => {
	const { profile, ...rest } = await request.json();
	if (profile) {
		await repositories.businessProfile.saveBusinessProfile(profile);
	}
	return json({ success: true });
};
