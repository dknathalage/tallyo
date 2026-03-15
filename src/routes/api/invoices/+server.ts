import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreateInvoiceSchema, SearchParamsSchema } from '$lib/validation/schemas.js';

export const GET: RequestHandler = ({ url }) => {
	const params = validate(SearchParamsSchema, {
		search: url.searchParams.get('search') || undefined
	});
	const status = url.searchParams.get('status') || undefined;
	repositories.invoices.markOverdueInvoices();
	return json(repositories.invoices.getInvoices(params.search, status));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { lineItems, ...invoiceData } = body;
	const validated = validate(CreateInvoiceSchema, invoiceData);
	validated.client_id = fkOrNull(validated.client_id) as number;
	validated.payer_id = fkOrNull(validated.payer_id);
	try {
		const id = await repositories.invoices.createInvoice(validated, lineItems ?? []);
		return json({ id }, { status: 201 });
	} catch (err) {
		dbError(err);
	}
};
