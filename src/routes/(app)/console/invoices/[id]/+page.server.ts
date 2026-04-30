import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ params }) => {
	const id = parseInt(params.id);
	const invoice = await repositories.invoices.getInvoice(id);
	if (!invoice) throw error(404, 'Invoice not found');
	return {
		invoice,
		lineItems: await repositories.invoices.getInvoiceLineItems(id),
		payments: await repositories.payments.getInvoicePayments(id),
		auditHistory: await repositories.audit.getEntityHistory('invoice', id)
	};
};
