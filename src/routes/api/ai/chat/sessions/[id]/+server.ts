import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { compactSession } from '$lib/server/ai/agent.js';

function parseId(raw: string | undefined): number {
	const n = Number(raw);
	if (!Number.isFinite(n) || n <= 0) throw error(400, 'invalid id');
	return n;
}

export const GET: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	const session = await repositories.aiChat.getSession(id);
	if (!session) throw error(404, 'not found');
	const messages = await repositories.aiChat.listMessages(id);
	const pending = await repositories.aiChat.listPendingToolCalls(id);
	return json({ session, messages, pending });
};

export const DELETE: RequestHandler = async ({ params }) => {
	const id = parseId(params.id);
	await repositories.aiChat.deleteSession(id);
	return json({ success: true });
};

export const POST: RequestHandler = async ({ params, request }) => {
	const id = parseId(params.id);
	const body = (await request.json().catch(() => ({}))) as { action?: unknown; title?: unknown };
	const action = typeof body.action === 'string' ? body.action : '';
	if (action === 'clear') {
		await repositories.aiChat.clearMessages(id);
		return json({ success: true });
	}
	if (action === 'compact') {
		await compactSession(id);
		return json({ success: true });
	}
	if (action === 'rename') {
		const title = typeof body.title === 'string' ? body.title : '';
		if (!title.trim()) throw error(400, 'title required');
		await repositories.aiChat.renameSession(id, title);
		return json({ success: true });
	}
	throw error(400, 'unknown action');
};
