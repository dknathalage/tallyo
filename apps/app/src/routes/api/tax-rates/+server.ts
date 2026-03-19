import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/postgres/index.js';
import { dbError } from '$lib/server/db-error.js';

export const GET: RequestHandler = async () => {
	return json(await repositories.taxRates.getTaxRates());
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	try {
		const id = await repositories.taxRates.createTaxRate(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
