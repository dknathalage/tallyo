import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/postgres/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreatePaymentSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = async ({ url }) => {
	const invoiceId = url.searchParams.get('invoiceId');
	if (!invoiceId) return json([]);
	return json(await repositories.payments.getInvoicePayments(parseInt(invoiceId)));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = await request.json();
	const validated = validate(CreatePaymentSchema, data);
	validated.invoice_id = fkOrNull(validated.invoice_id) as number;
	try {
		const id = await repositories.payments.createPayment(validated);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
