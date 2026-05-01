import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { BulkDeleteSchema, SearchParamsSchema } from '$lib/validation/schemas.js';
import type { CreatePayerInput } from '$lib/repositories/interfaces/types.js';

export const GET: RequestHandler = async ({ url }) => {
	const search = url.searchParams.get('search') ?? undefined;
	if (search && search.length > 255) throw error(400, 'Search query too long');
	const params = validate(SearchParamsSchema, { search });
	return json(await repositories.payers.getPayers(params.search));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json()) as { action?: string } & Record<string, unknown>;
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.payers.bulkDeletePayers(ids);
		} catch (err) {
			throw dbError(err);
		}
		return json({ success: true });
	}

	try {
		const id = await repositories.payers.createPayer(data as unknown as CreatePayerInput);
		return json({ id }, { status: 201 });
	} catch (err) {
		throw dbError(err);
	}
};
