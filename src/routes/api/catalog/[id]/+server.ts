import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const item = repositories.catalog.getCatalogItem(id);
	if (!item) throw error(404, 'Catalog item not found');
	return json(item);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	await repositories.catalog.updateCatalogItem(id, data);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.catalog.deleteCatalogItem(id);
	return json({ success: true });
};
