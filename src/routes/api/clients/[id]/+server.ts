import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const client = await repositories.clients.getClient(id);
	if (!client) throw error(404, 'Client not found');
	return json(client);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const data = (await request.json()) as Record<string, unknown>;
	data['pricing_tier_id'] = fkOrNull(data['pricing_tier_id']);
	data['payer_id'] = fkOrNull(data['payer_id']);
	try {
		await repositories.clients.updateClient(id, data as unknown as Parameters<typeof repositories.clients.updateClient>[1]);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.clients.deleteClient(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};
