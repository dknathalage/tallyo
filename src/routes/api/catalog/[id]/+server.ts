import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';
import type { UpdateCatalogItemInput } from '$lib/repositories/interfaces/types.js';

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const item = await repositories.catalog.getCatalogItem(id);
	if (!item) throw error(404, 'Catalog item not found');
	return json(item);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const data = (await request.json()) as UpdateCatalogItemInput;
	try {
		await repositories.catalog.updateCatalogItem(id, data);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.catalog.deleteCatalogItem(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};
