import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreateClientSchema, BulkDeleteSchema, SearchParamsSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = ({ url }) => {
	const params = validate(SearchParamsSchema, {
		search: url.searchParams.get('search') || undefined
	});
	return json(repositories.clients.getClients(params.search));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.clients.bulkDeleteClients(ids);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}

	const validated = validate(CreateClientSchema, data);
	validated.pricing_tier_id = fkOrNull(validated.pricing_tier_id);
	validated.payer_id = fkOrNull(validated.payer_id);

	try {
		const id = await repositories.clients.createClient(validated);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
