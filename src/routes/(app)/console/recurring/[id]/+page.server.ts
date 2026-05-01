import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const template = await repositories.recurringTemplates.getRecurringTemplate(id);
	if (!template) error(404, 'Template not found');
	return {
		template,
		clients: (await repositories.clients.getClients(undefined, { limit: 200 })).data
	};
};
