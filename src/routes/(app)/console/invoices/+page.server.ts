import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = () => {
	repositories.invoices.markOverdueInvoices();
	return {
		invoices: repositories.invoices.getInvoices(),
		dueTemplatesCount: repositories.recurringTemplates.getDueTemplates().length
	};
};
