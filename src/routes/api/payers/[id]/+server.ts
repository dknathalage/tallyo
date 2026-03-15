import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const payer = repositories.payers.getPayer(id);
	if (!payer) throw error(404, 'Payer not found');
	return json(payer);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	await repositories.payers.updatePayer(id, data);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.payers.deletePayer(id);
	return json({ success: true });
};
