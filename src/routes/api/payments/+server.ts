import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreatePaymentSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = async ({ url }) => {
	const invoiceId = url.searchParams.get('invoiceId');
	if (!invoiceId) return json([]);
	return json(await repositories.payments.getInvoicePayments(parseInt(invoiceId)));
};

export const POST: RequestHandler = async ({ request }) => {
	const data = (await request.json()) as Record<string, unknown>;
	const validated = validate(CreatePaymentSchema, data);
	const invoice_id = fkOrNull(validated.invoice_id);
	try {
		const input = {
			invoice_id,
			amount: validated.amount,
			payment_date: validated.payment_date,
			...(validated.method !== undefined && { method: validated.method }),
			...(validated.notes !== undefined && { notes: validated.notes })
		};
		const id = await repositories.payments.createPayment(input as unknown as Parameters<typeof repositories.payments.createPayment>[0]);
		return json({ id }, { status: 201 });
	} catch (err) {
		throw dbError(err);
	}
};
