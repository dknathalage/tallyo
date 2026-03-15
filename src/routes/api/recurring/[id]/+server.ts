import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const template = repositories.recurringTemplates.getRecurringTemplate(id);
	if (!template) throw error(404, 'Template not found');
	return json(template);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	await repositories.recurringTemplates.updateRecurringTemplate(id, data);
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.recurringTemplates.deleteRecurringTemplate(id);
	return json({ success: true });
};

export const PATCH: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	const invoiceId = await repositories.recurringTemplates.createInvoiceFromTemplate(id);
	return json({ invoiceId });
};
