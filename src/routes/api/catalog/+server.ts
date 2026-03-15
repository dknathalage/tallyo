import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { BulkDeleteSchema, SearchParamsSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = ({ url }) => {
	const params = validate(SearchParamsSchema, {
		search: url.searchParams.get('search') || undefined
	});
	const category = url.searchParams.get('category') || undefined;
	return json(repositories.catalog.getCatalogItems(params.search, category));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.catalog.bulkDeleteCatalogItems(ids);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}

	try {
		const id = await repositories.catalog.createCatalogItem(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
