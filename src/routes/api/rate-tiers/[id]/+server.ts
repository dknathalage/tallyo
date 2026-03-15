import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	if (isNaN(id)) error(400, 'Invalid tier ID');

	const data = await request.json();
	if (!data.name?.trim()) error(400, 'Tier name is required');

	try {
		await repositories.rateTiers.updateRateTier(id, data);
		return json({ success: true });
	} catch (err: unknown) {
		const msg = err instanceof Error ? err.message : String(err);
		if (msg.includes('UNIQUE constraint failed')) {
			error(409, `A rate tier named "${data.name}" already exists`);
		}
		throw err;
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	if (isNaN(id)) error(400, 'Invalid tier ID');

	try {
		await repositories.rateTiers.deleteRateTier(id);
		return json({ success: true });
	} catch (err: unknown) {
		const msg = err instanceof Error ? err.message : String(err);
		if (msg.includes('Cannot delete the last tier')) {
			error(400, msg);
		}
		throw err;
	}
};
