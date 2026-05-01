import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { dbError } from '$lib/server/db-error.js';
import type { UpdateRecurringTemplateInput } from '$lib/repositories/interfaces/RecurringTemplateRepository.js';

function parseId(raw: string): number {
	const id = parseInt(raw, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	return id;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const template = await repositories.recurringTemplates.getRecurringTemplate(id);
	if (!template) throw error(404, 'Template not found');
	return json(template);
};

export const PUT: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const data = (await request.json()) as UpdateRecurringTemplateInput;
	try {
		await repositories.recurringTemplates.updateRecurringTemplate(id, data);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		await repositories.recurringTemplates.deleteRecurringTemplate(id);
	} catch (err) {
		throw dbError(err);
	}
	return json({ success: true });
};

export const PATCH: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	try {
		const invoiceId = await repositories.recurringTemplates.createInvoiceFromTemplate(id);
		return json({ invoiceId });
	} catch (err) {
		throw dbError(err);
	}
};
