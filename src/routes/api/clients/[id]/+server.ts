import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const client = repositories.clients.getClient(id);
	if (!client) throw error(404, 'Client not found');
	return json(client);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	await repositories.clients.updateClient(id, data);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.clients.deleteClient(id);
	return json({ success: true });
};
