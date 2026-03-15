import { json, error } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';
import type { RequestHandler } from './$types.js';

export const GET: RequestHandler = ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const session = repositories.aiChat.getSession(id);
	if (!session) throw error(404, 'Session not found');
	return json({ ...session, messages: repositories.aiChat.getSessionMessages(id) });
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	const { title } = await request.json();
	if (title) repositories.aiChat.updateSessionTitle(id, title);
	return json({ success: true });
};

export const DELETE: RequestHandler = ({ params }) => {
	const id = parseInt(params.id, 10);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'Invalid ID');
	repositories.aiChat.deleteSession(id);
	return json({ success: true });
};
