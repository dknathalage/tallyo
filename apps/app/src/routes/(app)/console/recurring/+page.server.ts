import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async () => {
	return {
		templates: await repositories.recurringTemplates.getRecurringTemplates()
	};
};
