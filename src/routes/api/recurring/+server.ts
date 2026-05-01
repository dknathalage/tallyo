import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';
import type { CreateRecurringTemplateInput } from '$lib/repositories/interfaces/RecurringTemplateRepository.js';

export const GET: RequestHandler = async ({ url }) => {
	const all = url.searchParams.get('all') === 'true';
	return json(await repositories.recurringTemplates.getRecurringTemplates(!all));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = (await request.json()) as CreateRecurringTemplateInput;
	try {
		const id = await repositories.recurringTemplates.createRecurringTemplate(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		throw dbError(err);
	}
};
