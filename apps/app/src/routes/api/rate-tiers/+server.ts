import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/postgres/index.js';

export const GET: RequestHandler = async () => {
	return json(await repositories.rateTiers.getRateTiers());
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();

	if (!data.name?.trim()) {
		error(400, 'Tier name is required');
	}

	try {
		const id = await repositories.rateTiers.createRateTier(data);
		return json({ id }, { status: 201 });
	} catch (err: unknown) {
		const msg = err instanceof Error ? err.message : String(err);
		if (msg.includes('UNIQUE constraint failed')) {
			error(409, `A rate tier named "${data.name}" already exists`);
		}
		throw err;
	}
};
