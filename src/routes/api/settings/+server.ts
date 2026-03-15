import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';

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
		try {
			await repositories.businessProfile.saveBusinessProfile(profile);
		} catch (err) {
			dbError(err);
		}
	}
	return json({ success: true });
};
