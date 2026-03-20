import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';

export const GET: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const template = await repositories.recurringTemplates.getRecurringTemplate(id);
	if (!template) throw error(404, 'Template not found');
	return json(template);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const data = await request.json();
	try {
		await repositories.recurringTemplates.updateRecurringTemplate(id, data);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	try {
		await repositories.recurringTemplates.deleteRecurringTemplate(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};

export const PATCH: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	try {
		const invoiceId = await repositories.recurringTemplates.createInvoiceFromTemplate(id);
		return json({ invoiceId });
	} catch (err) {
		dbError(err);
	}
};
