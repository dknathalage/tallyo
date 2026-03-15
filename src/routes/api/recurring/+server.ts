import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ url }) => {
	const all = url.searchParams.get('all') === 'true';
	return json(repositories.recurringTemplates.getRecurringTemplates(!all));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	const id = await repositories.recurringTemplates.createRecurringTemplate(data);
	return json({ id }, { status: 201 });
};
