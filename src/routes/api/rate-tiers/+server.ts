import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = () => {
	return json(repositories.rateTiers.getRateTiers());
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	const id = await repositories.rateTiers.createRateTier(data);
	return json({ id }, { status: 201 });
};
