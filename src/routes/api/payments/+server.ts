import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ url }) => {
	const invoiceId = url.searchParams.get('invoiceId');
	if (!invoiceId) return json([]);
	return json(repositories.payments.getInvoicePayments(parseInt(invoiceId)));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	data.invoice_id = fkOrNull(data.invoice_id);
	try {
		const id = await repositories.payments.createPayment(data);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
