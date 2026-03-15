import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	return json(repositories.payers.getPayers(search));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		await repositories.payers.bulkDeletePayers(data.ids ?? []);
		return json({ success: true });
	}

	const id = await repositories.payers.createPayer(data);
	return json({ id }, { status: 201 });
};
