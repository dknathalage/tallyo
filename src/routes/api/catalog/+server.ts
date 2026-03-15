import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	const category = url.searchParams.get('category') || undefined;
	return json(repositories.catalog.getCatalogItems(search, category));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		await repositories.catalog.bulkDeleteCatalogItems(data.ids ?? []);
		return json({ success: true });
	}

	const id = await repositories.catalog.createCatalogItem(data);
	return json({ id }, { status: 201 });
};
