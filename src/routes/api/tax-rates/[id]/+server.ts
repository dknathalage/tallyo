import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	await repositories.taxRates.updateTaxRate(id, data);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.taxRates.deleteTaxRate(id);
	return json({ success: true });
};
