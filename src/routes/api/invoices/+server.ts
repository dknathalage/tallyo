import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ url }) => {
	const search = url.searchParams.get('search') || undefined;
	const status = url.searchParams.get('status') || undefined;
	repositories.invoices.markOverdueInvoices();
	return json(repositories.invoices.getInvoices(search, status));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { lineItems, ...invoiceData } = body;
	invoiceData.client_id = fkOrNull(invoiceData.client_id);
	invoiceData.payer_id = fkOrNull(invoiceData.payer_id);
	try {
		const id = await repositories.invoices.createInvoice(invoiceData, lineItems ?? []);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
