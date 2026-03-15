import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id);
	const template = repositories.recurringTemplates.getRecurringTemplate(id);
	if (!template) throw error(404, 'Template not found');
	return json(template);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id);
	const data = await request.json();
	try {
		await repositories.recurringTemplates.updateRecurringTemplate(id, data);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	try {
		await repositories.recurringTemplates.deleteRecurringTemplate(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const PATCH: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	try {
		const invoiceId = await repositories.recurringTemplates.createInvoiceFromTemplate(id);
		return json({ invoiceId });
	} catch (err) {
		dbError(err);
	}
};
