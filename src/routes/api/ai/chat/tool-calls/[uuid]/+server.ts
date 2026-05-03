import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { approveToolCall, rejectToolCall } from '$lib/server/ai/agent.js';

export const POST: RequestHandler = async ({ params, request }) => {
	const uuid = params.uuid ?? '';
	if (!uuid) throw error(400, 'uuid required');
	const body = (await request.json().catch(() => ({}))) as { action?: unknown };
	const action = typeof body.action === 'string' ? body.action : '';
	if (action === 'approve') {
		const result = await approveToolCall(uuid);
		if (result.error) throw error(400, result.error);
		return json({ success: true, result: result.result });
	}
	if (action === 'reject') {
		await rejectToolCall(uuid);
		return json({ success: true });
	}
	throw error(400, 'unknown action');
};
