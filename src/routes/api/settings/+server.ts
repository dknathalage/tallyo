import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';

export const GET: RequestHandler = async () => {
	return json({
		profile: await repositories.businessProfile.getBusinessProfile(),
		taxRates: await repositories.taxRates.getTaxRates(),
		columnMappings: await repositories.columnMappings.getColumnMappings()
	});
};

export const POST: RequestHandler = async ({ request }) => {
	const { profile } = await request.json();
	if (profile) {
		try {
			await repositories.businessProfile.saveBusinessProfile(profile);
		} catch (err) {
			dbError(err);
		}
	}
	return json({ success: true });
};
