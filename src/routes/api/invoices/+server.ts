import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import { validate } from '$lib/validation/validate.js';
import { CreateInvoiceSchema, SearchParamsSchema } from '$lib/validation/schemas.js';
import type { CreateInvoiceInput, LineItemInput } from '$lib/repositories/interfaces/types.js';

type InvoicePostBody = { lineItems?: LineItemInput[] } & Record<string, unknown>;

export const GET: RequestHandler = async ({ url }) => {
	const search = url.searchParams.get('search') ?? undefined;
	if (search && search.length > 255) throw error(400, 'Search query too long');
	const params = validate(SearchParamsSchema, { search });
	const status = url.searchParams.get('status') ?? undefined;
	const page = parseInt(url.searchParams.get('page') ?? '1', 10);
	const limit = Math.min(parseInt(url.searchParams.get('limit') ?? '50', 10), 200);
	await repositories.invoices.markOverdueInvoices();
	return json(await repositories.invoices.getInvoices(params.search, status, { page, limit }));
};

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json()) as InvoicePostBody;
	const { lineItems, ...invoiceData } = body;
	const validated = validate(CreateInvoiceSchema, invoiceData);
	(validated as { client_id: number | null }).client_id = fkOrNull(validated.client_id);
	(validated as { payer_id: number | null }).payer_id = fkOrNull(validated.payer_id);
	try {
		const id = await repositories.invoices.createInvoice(validated as unknown as CreateInvoiceInput, lineItems ?? []);
		return json({ id }, { status: 201 });
	} catch (err) {
		throw dbError(err);
	}
};
