import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	return json(repositories.clients.getClients(search));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		await repositories.clients.bulkDeleteClients(data.ids ?? []);
		return json({ success: true });
	}

	const id = await repositories.clients.createClient(data);
	return json({ id }, { status: 201 });
};
