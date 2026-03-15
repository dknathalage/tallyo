import { json } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/sqlite/index.js';
import type { RequestHandler } from './$types.js';

export const GET: RequestHandler = () => json(repositories.aiChat.getSessions());

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json().catch(() => ({}));
	const id = repositories.aiChat.createSession(body.title);
	return json(repositories.aiChat.getSession(id), { status: 201 });
};
