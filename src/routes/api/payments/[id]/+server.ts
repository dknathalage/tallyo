import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	await repositories.payments.deletePayment(id);
	return json({ success: true });
};
