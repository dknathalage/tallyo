import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreateEstimateSchema, BulkDeleteSchema, SearchParamsSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = ({ url }) => {
	const params = validate(SearchParamsSchema, {
		search: url.searchParams.get('search') || undefined
	});
	const status = url.searchParams.get('status') || undefined;
	return json(repositories.estimates.getEstimates(params.search, status));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.estimates.bulkDeleteEstimates(ids);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}
	if (action === 'bulk-status') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.estimates.bulkUpdateEstimateStatus(ids, data.status);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}

	const { lineItems, ...estimateData } = data;
	const validated = validate(CreateEstimateSchema, estimateData);
	validated.client_id = fkOrNull(validated.client_id) as number;
	validated.payer_id = fkOrNull(validated.payer_id);
	try {
		const id = await repositories.estimates.createEstimate(validated, lineItems ?? []);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
