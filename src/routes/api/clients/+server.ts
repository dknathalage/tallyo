import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreateClientSchema, BulkDeleteSchema, SearchParamsSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = async ({ url }) => {
	const search = url.searchParams.get('search') ?? undefined;
	if (search && search.length > 255) throw error(400, 'Search query too long');
	const params = validate(SearchParamsSchema, { search });
	const page = parseInt(url.searchParams.get('page') ?? '1', 10);
	const limit = Math.min(parseInt(url.searchParams.get('limit') ?? '50', 10), 200);
	return json(await repositories.clients.getClients(params.search, { page, limit }));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json()) as { action?: string } & Record<string, unknown>;
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		const { ids } = validate(BulkDeleteSchema, data);
		try {
			await repositories.clients.bulkDeleteClients(ids);
		} catch (err) {
			throw dbError(err);
		}
		return json({ success: true });
	}

	const validated = validate(CreateClientSchema, data);
	const pricing_tier_id = fkOrNull(validated.pricing_tier_id);
	const payer_id = fkOrNull(validated.payer_id);

	try {
		const input = {
			name: validated.name,
			...(validated.email !== undefined && { email: validated.email }),
			...(validated.phone !== undefined && { phone: validated.phone }),
			...(validated.address !== undefined && { address: validated.address }),
			pricing_tier_id,
			payer_id,
			...(validated.metadata !== undefined && { metadata: validated.metadata })
		};
		const id = await repositories.clients.createClient(input);
		return json({ id }, { status: 201 });
	} catch (err) {
		throw dbError(err);
	}
};
