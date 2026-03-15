import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const item = repositories.catalog.getCatalogItem(id);
	if (!item) throw error(404, 'Catalog item not found');
	return json(item);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const data = await request.json();
	try {
		await repositories.catalog.updateCatalogItem(id, data);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	try {
		await repositories.catalog.deleteCatalogItem(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};
