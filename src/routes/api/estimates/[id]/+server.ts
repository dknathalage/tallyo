import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

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
	estimateData.client_id = fkOrNull(estimateData.client_id);
	estimateData.payer_id = fkOrNull(estimateData.payer_id);
	try {
		await repositories.estimates.updateEstimate(id, estimateData, lineItems ?? []);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	try {
		await repositories.estimates.deleteEstimate(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const { action, ...data } = await request.json();

	if (action === 'status') {
		try {
			await repositories.estimates.updateEstimateStatus(id, data.status);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}
	if (action === 'duplicate') {
		try {
			const result = await repositories.estimates.duplicateEstimate(id);
			return json(result);
		} catch (err) {
			dbError(err);
		}
	}
	if (action === 'convert') {
		try {
			const result = await repositories.estimates.convertEstimateToInvoice(id);
			return json(result);
		} catch (err) {
			dbError(err);
		}
	}
	throw error(400, 'Unknown action');
};
