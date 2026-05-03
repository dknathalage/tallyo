import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const GET: RequestHandler = async ({ params }) => {
	const id = Number(params.id);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'invalid id');
	const calls = await repositories.aiChat.listToolCalls(id);
	return json({ calls });
};
