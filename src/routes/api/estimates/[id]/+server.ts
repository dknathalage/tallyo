import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const estimate = repositories.estimates.getEstimate(id);
	if (!estimate) throw error(404, 'Estimate not found');
	return json(estimate);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const body = await request.json();
	const { lineItems, ...estimateData } = body;
	await repositories.estimates.updateEstimate(id, estimateData, lineItems ?? []);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.estimates.deleteEstimate(id);
	return json({ success: true });
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const { action, ...data } = await request.json();

	if (action === 'status') {
		await repositories.estimates.updateEstimateStatus(id, data.status);
		return json({ success: true });
	}
	if (action === 'duplicate') {
		const result = await repositories.estimates.duplicateEstimate(id);
		return json(result);
	}
	if (action === 'convert') {
		const result = await repositories.estimates.convertEstimateToInvoice(id);
		return json(result);
	}
	throw error(400, 'Unknown action');
};
