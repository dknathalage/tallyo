import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ url }) => {
	const page = parseInt(url.searchParams.get('page') || '1', 10);
	const limit = Math.min(parseInt(url.searchParams.get('limit') || '50', 10), 200);
	await repositories.invoices.markOverdueInvoices();
	return {
		invoicesResult: await repositories.invoices.getInvoices(undefined, undefined, { page, limit }),
		dueTemplatesCount: (await repositories.recurringTemplates.getDueTemplates()).length
	};
};
