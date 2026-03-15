import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	const status = url.searchParams.get('status') || undefined;
	return json(repositories.estimates.getEstimates(search, status));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		try {
			await repositories.estimates.bulkDeleteEstimates(data.ids ?? []);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}
	if (action === 'bulk-status') {
		try {
			await repositories.estimates.bulkUpdateEstimateStatus(data.ids ?? [], data.status);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}

	const { lineItems, ...estimateData } = data;
	estimateData.client_id = fkOrNull(estimateData.client_id);
	estimateData.payer_id = fkOrNull(estimateData.payer_id);
	try {
		const id = await repositories.estimates.createEstimate(estimateData, lineItems ?? []);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
