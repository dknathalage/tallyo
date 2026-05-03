import { error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { repositories } from '$lib/repositories/index.js';
import { runAgentTurn } from '$lib/server/ai/agent.js';
import type { AiProgressEvent } from '$lib/server/ai/llm.js';
import { log } from '$lib/server/logger.js';

const l = log('api:ai:messages');

export const POST: RequestHandler = async ({ params, request }) => {
	const id = Number(params.id);
	if (!Number.isFinite(id) || id <= 0) throw error(400, 'invalid id');
	const session = await repositories.aiChat.getSession(id);
	if (!session) throw error(404, 'session not found');

	const body = (await request.json()) as {
		message?: unknown;
		route?: unknown;
		continuation?: unknown;
		editFromMessageId?: unknown;
	};
	const message = typeof body.message === 'string' ? body.message : '';
	const route = typeof body.route === 'string' ? body.route : undefined;
	const continuation = body.continuation === true;
	const editFromMessageId =
		typeof body.editFromMessageId === 'number' && Number.isFinite(body.editFromMessageId)
			? body.editFromMessageId
			: null;
	if (editFromMessageId !== null) {
		await repositories.aiChat.deleteMessagesFrom(id, editFromMessageId);
	}
	if (!continuation) {
		if (!message.trim()) throw error(400, 'message required');
		if (message.length > 8000) throw error(400, 'message too long');
	}

	const enc = new TextEncoder();
	const stream = new ReadableStream<Uint8Array>({
		async start(controller) {
			const send = (obj: unknown): void => {
				controller.enqueue(enc.encode(JSON.stringify(obj) + '\n'));
			};
			try {
				const opts: Parameters<typeof runAgentTurn>[0] = {
					sessionId: id,
					userMessage: message,
					emit: send,
					onProgress: (e: AiProgressEvent) => send(e),
					continuation
				};
				if (route) opts.routeContext = route;
				await runAgentTurn(opts);
			} catch (e) {
				const msg = e instanceof Error ? e.message : 'agent failed';
				l.error('stream errored', { sessionId: id, error: msg, stack: e instanceof Error ? e.stack : undefined });
				send({ type: 'error', message: msg });
			} finally {
				controller.close();
			}
		}
	});

	return new Response(stream, {
		headers: {
			'Content-Type': 'application/x-ndjson',
			'Cache-Control': 'no-store',
			'X-Accel-Buffering': 'no'
		}
	});
};
