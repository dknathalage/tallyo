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
	return json(repositories.payers.getPayers(params.search));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.payers.bulkDeletePayers(ids);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}

	try {
		const id = await repositories.payers.createPayer(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
