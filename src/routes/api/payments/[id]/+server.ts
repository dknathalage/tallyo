import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';
import { dbError } from '$lib/server/db-error.js';

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseInt(params.id);
	try {
		await repositories.payments.deletePayment(id);
		return json({ success: true });
	} catch (err) {
		dbError(err);
	}
};
