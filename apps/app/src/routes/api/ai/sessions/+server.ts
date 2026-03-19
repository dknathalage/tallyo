import { json } from '@sveltejs/kit';
import { repositories } from '$lib/repositories/postgres/index.js';
import type { RequestHandler } from './$types.js';

export const GET: RequestHandler = async () => json(await repositories.aiChat.getSessions());

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json().catch(() => ({}));
	const id = await repositories.aiChat.createSession(body.title);
	return json(await repositories.aiChat.getSession(id), { status: 201 });
};
