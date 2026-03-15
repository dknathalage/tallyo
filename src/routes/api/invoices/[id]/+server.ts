import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const invoice = repositories.invoices.getInvoice(id);
	if (!invoice) throw error(404, 'Invoice not found');
	return json(invoice);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const body = await request.json();
	const { lineItems, ...invoiceData } = body;
	await repositories.invoices.updateInvoice(id, invoiceData, lineItems ?? []);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.invoices.deleteInvoice(id);
	return json({ success: true });
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const { action, ...data } = await request.json();

	if (action === 'status') {
		await repositories.invoices.updateInvoiceStatus(id, data.status);
		return json({ success: true });
	}
	if (action === 'duplicate') {
		const newId = await repositories.invoices.duplicateInvoice(id);
		return json({ id: newId });
	}
	throw error(400, 'Unknown action');
};
