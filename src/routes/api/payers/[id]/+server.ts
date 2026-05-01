import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';
import type { UpdatePayerInput } from '$lib/repositories/interfaces/types.js';

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const payer = await repositories.payers.getPayer(id);
	if (!payer) throw error(404, 'Payer not found');
	return json(payer);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const data = (await request.json()) as UpdatePayerInput;
	try {
		await repositories.payers.updatePayer(id, data);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.payers.deletePayer(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};
