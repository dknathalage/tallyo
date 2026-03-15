import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	try {
		await repositories.taxRates.updateTaxRate(id, data);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	try {
		await repositories.taxRates.deleteTaxRate(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};
