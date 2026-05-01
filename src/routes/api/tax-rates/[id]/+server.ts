import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';

interface TaxRateInput {
	name: string;
	rate: number;
	is_default?: boolean;
}

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const data = (await request.json()) as TaxRateInput;
	try {
		await repositories.taxRates.updateTaxRate(id, data);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.taxRates.deleteTaxRate(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};
