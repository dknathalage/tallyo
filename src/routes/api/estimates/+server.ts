import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	const status = url.searchParams.get('status') || undefined;
	return json(repositories.estimates.getEstimates(search, status));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		await repositories.estimates.bulkDeleteEstimates(data.ids ?? []);
		return json({ success: true });
	}
	if (action === 'bulk-status') {
		await repositories.estimates.bulkUpdateEstimateStatus(data.ids ?? [], data.status);
		return json({ success: true });
	}

	const { lineItems, ...estimateData } = data;
	const id = await repositories.estimates.createEstimate(estimateData, lineItems ?? []);
	return json({ id }, { status: 201 });
};
