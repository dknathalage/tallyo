import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';

interface TaxRateInput {
	name: string;
	rate: number;
	is_default?: boolean;
}

export const GET: RequestHandler = async () => {
	return json(await repositories.taxRates.getTaxRates());
};

export const POST: RequestHandler = async ({ request }) => {
	const data = (await request.json()) as TaxRateInput;
	try {
		const id = await repositories.taxRates.createTaxRate(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		throw dbError(err);
	}
};
