import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import type { UpdateEstimateInput, LineItemInput } from '$lib/repositories/interfaces/types.js';

type EstimatePutBody = UpdateEstimateInput & {
	lineItems?: LineItemInput[];
	payer_id?: number | null;
};
type EstimatePatchBody = { action?: string; status?: string } & Record<string, unknown>;

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const estimate = await repositories.estimates.getEstimate(id);
	if (!estimate) throw error(404, 'Estimate not found');
	return json(estimate);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const body = (await request.json()) as EstimatePutBody;
	const { lineItems, ...estimateData } = body;
	const client_id = fkOrNull(estimateData.client_id);
	if (client_id === null) throw error(400, 'Client ID is required');
	estimateData.client_id = client_id;
	estimateData.payer_id = fkOrNull(estimateData.payer_id);
	try {
		await repositories.estimates.updateEstimate(id, estimateData, lineItems ?? []);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.estimates.deleteEstimate(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const { action, ...data } = (await request.json()) as EstimatePatchBody;

	if (action === 'status') {
		const status = typeof data.status === 'string' ? data.status : '';
		try {
			await repositories.estimates.updateEstimateStatus(id, status);
		} catch (err) {
			throw dbError(err);
		}
		return json({ success: true });
	}
	if (action === 'duplicate') {
		try {
			const result = await repositories.estimates.duplicateEstimate(id);
			return json(result);
		} catch (err) {
			throw dbError(err);
		}
	}
	if (action === 'convert') {
		try {
			const result = await repositories.estimates.convertEstimateToInvoice(id);
			return json(result);
		} catch (err) {
			throw dbError(err);
		}
	}
	throw error(400, 'Unknown action');
};
