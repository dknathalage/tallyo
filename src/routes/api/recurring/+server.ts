import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ url }) => {
	const all = url.searchParams.get('all') === 'true';
	return json(repositories.recurringTemplates.getRecurringTemplates(!all));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	try {
		const id = await repositories.recurringTemplates.createRecurringTemplate(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
