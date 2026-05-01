import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';
import type { UpdateInvoiceInput, LineItemInput } from '$lib/repositories/interfaces/types.js';

type InvoicePutBody = UpdateInvoiceInput & {
	lineItems?: LineItemInput[];
	payer_id?: number | null;
};
type InvoicePatchBody = { action?: string; status?: string } & Record<string, unknown>;

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const invoice = await repositories.invoices.getInvoice(id);
	if (!invoice) throw error(404, 'Invoice not found');
	return json(invoice);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const body = (await request.json()) as InvoicePutBody;
	const { lineItems, ...invoiceData } = body;
	const client_id = fkOrNull(invoiceData.client_id);
	if (client_id === null) throw error(400, 'Client ID is required');
	invoiceData.client_id = client_id;
	invoiceData.payer_id = fkOrNull(invoiceData.payer_id);
	try {
		await repositories.invoices.updateInvoice(id, invoiceData, lineItems ?? []);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.invoices.deleteInvoice(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const { action, ...data } = (await request.json()) as InvoicePatchBody;

	if (action === 'status') {
		const status = typeof data.status === 'string' ? data.status : '';
		try {
			await repositories.invoices.updateInvoiceStatus(id, status);
		} catch (err) {
			throw dbError(err);
		}
		return json({ success: true });
	}
	if (action === 'duplicate') {
		try {
			const newId = await repositories.invoices.duplicateInvoice(id);
			return json({ id: newId });
		} catch (err) {
			throw dbError(err);
		}
	}
	throw error(400, 'Unknown action');
};
