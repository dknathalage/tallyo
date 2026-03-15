import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	return json(repositories.clients.getClients(search));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { action, ...data } = body;

	if (action === 'bulk-delete') {
		try {
			await repositories.clients.bulkDeleteClients(data.ids ?? []);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}

	// Normalize FK fields — forms send 0 for "none", which breaks FK constraint
	data.pricing_tier_id = fkOrNull(data.pricing_tier_id);
	data.payer_id = fkOrNull(data.payer_id);

	try {
		const id = await repositories.clients.createClient(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
