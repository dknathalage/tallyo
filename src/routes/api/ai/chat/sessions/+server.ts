import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const GET: RequestHandler = async () => {
	const sessions = await repositories.aiChat.listSessions();
	return json({ sessions });
};

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json().catch(() => ({}))) as { title?: unknown };
	const title = typeof body.title === 'string' ? body.title : 'New chat';
	if (title.length > 200) throw error(400, 'title too long');
	const session = await repositories.aiChat.createSession(title);
	return json({ session }, { status: 201 });
};
