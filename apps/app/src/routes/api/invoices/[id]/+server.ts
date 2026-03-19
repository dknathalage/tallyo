import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/postgres/index.js';
import { dbError, fkOrNull } from '$lib/server/db-error.js';

export const GET: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const invoice = await repositories.invoices.getInvoice(id);
	if (!invoice) throw error(404, 'Invoice not found');
	return json(invoice);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const body = await request.json();
	const { lineItems, ...invoiceData } = body;
	invoiceData.client_id = fkOrNull(invoiceData.client_id);
	invoiceData.payer_id = fkOrNull(invoiceData.payer_id);
	try {
		await repositories.invoices.updateInvoice(id, invoiceData, lineItems ?? []);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	try {
		await repositories.invoices.deleteInvoice(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const { action, ...data } = await request.json();

	if (action === 'status') {
		try {
			await repositories.invoices.updateInvoiceStatus(id, data.status);
			return json({ success: true });
		} catch (err) {
			dbError(err);
		}
	}
	if (action === 'duplicate') {
		try {
			const newId = await repositories.invoices.duplicateInvoice(id);
			return json({ id: newId });
		} catch (err) {
			dbError(err);
		}
	}
	throw error(400, 'Unknown action');
};
