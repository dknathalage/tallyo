import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

export const GET: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const client = await repositories.clients.getClient(id);
	if (!client) throw error(404, 'Client not found');
	return json(client);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const data = await request.json();
	data.pricing_tier_id = fkOrNull(data.pricing_tier_id);
	data.payer_id = fkOrNull(data.payer_id);
	try {
		await repositories.clients.updateClient(id, data);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	try {
		await repositories.clients.deleteClient(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};
