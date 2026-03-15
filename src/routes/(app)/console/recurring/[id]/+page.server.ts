import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ params }) => {
	const id = parseInt(params.id);
	const template = repositories.recurringTemplates.getRecurringTemplate(id);
	if (!template) throw error(404, 'Template not found');
	return {
		template,
		clients: repositories.clients.getClients(undefined, { limit: 200 }).data
	};
};
