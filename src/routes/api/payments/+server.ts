import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ url }) => {
	const invoiceId = url.searchParams.get('invoiceId');
	if (!invoiceId) return json([]);
	return json(repositories.payments.getInvoicePayments(parseInt(invoiceId)));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	const id = await repositories.payments.createPayment(data);
	return json({ id }, { status: 201 });
};
