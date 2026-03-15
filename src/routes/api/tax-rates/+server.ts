import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = () => {
	return json(repositories.taxRates.getTaxRates());
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	const id = await repositories.taxRates.createTaxRate(data);
	return json({ id }, { status: 201 });
};
