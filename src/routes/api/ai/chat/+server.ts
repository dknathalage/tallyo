import { error } from '@sveltejs/kit';
import Anthropic from '@anthropic-ai/sdk';
import { repositories } from '$lib/repositories/index.js';
import { AI_TOOLS, SYSTEM_PROMPT, executeTool } from '$lib/server/ai-tools.js';
import type { RequestHandler } from './$types.js';

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json();
	const { session_id, message } = body;
	if (!session_id || !message) throw error(400, 'session_id and message required');

	// Get API key from business profile metadata
	const profile = await repositories.businessProfile.getBusinessProfile();
	let apiKey: string | undefined;
	try {
		const m = JSON.parse(profile?.metadata ?? '{}') as Record<string, string>;
		apiKey = m['anthropic_api_key'];
	} catch {
		/* noop */
	}

	// Persist user message
	await repositories.aiChat.addMessage({ session_id, role: 'user', content: message });

	// Build full conversation history
	const history = await repositories.aiChat.getSessionMessages(session_id);
	const encoder = new TextEncoder();

	function sse(ctrl: ReadableStreamDefaultController, event: string, data: unknown) {
		ctrl.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
	}

	const stream = new ReadableStream({
		async start(ctrl) {
			if (!apiKey) {
				sse(ctrl, 'error', {
					message:
						'No Anthropic API key configured. Please add it in Settings → AI Assistant.'
				});
				ctrl.close();
				return;
			}

			const anthropic = new Anthropic({ apiKey });
			const assistantMsgId = await repositories.aiChat.addMessage({
				session_id,
				role: 'assistant',
				content: '',
				is_streaming: 1
			});

			let fullContent = '';
			const allToolCalls: unknown[] = [];
			const allToolResults: unknown[] = [];
			let msgs: Anthropic.MessageParam[] = history.map((m) => ({
				role: m.role as 'user' | 'assistant',
				content: m.content
			}));
			const MAX_ITER = 10;

			try {
				for (let iter = 0; iter < MAX_ITER; iter++) {
					const msgStream = anthropic.messages.stream({
						model: 'claude-opus-4-5',
						max_tokens: 4096,
						system: SYSTEM_PROMPT,
						tools: AI_TOOLS,
						messages: msgs
					});

					let iterText = '';
					const toolBlocks: Array<{
						id: string;
						name: string;
						input: Record<string, unknown>;
					}> = [];
					let curTool: { id: string; name: string; inputJson: string } | null = null;

					for await (const ev of msgStream) {
						if (
							ev.type === 'content_block_start' &&
							ev.content_block.type === 'tool_use'
						) {
							curTool = {
								id: ev.content_block.id,
								name: ev.content_block.name,
								inputJson: ''
							};
							sse(ctrl, 'tool_start', {
								id: ev.content_block.id,
								name: ev.content_block.name
							});
						} else if (ev.type === 'content_block_delta') {
							if (ev.delta.type === 'text_delta') {
								iterText += ev.delta.text;
								fullContent += ev.delta.text;
								sse(ctrl, 'text_delta', { delta: ev.delta.text });
							} else if (ev.delta.type === 'input_json_delta' && curTool) {
								curTool.inputJson += ev.delta.partial_json;
							}
						} else if (ev.type === 'content_block_stop' && curTool) {
							try {
								toolBlocks.push({
									id: curTool.id,
									name: curTool.name,
									input: JSON.parse(curTool.inputJson || '{}') as Record<string, unknown>
								});
							} catch {
								/* noop */
							}
							curTool = null;
						}
					}

					const final = await msgStream.finalMessage();
					if (final?.stop_reason === 'tool_use' && toolBlocks.length > 0) {
						const toolResults: Anthropic.ToolResultBlockParam[] = [];
						for (const t of toolBlocks) {
							const result = await executeTool(t.name, t.input);
							const str = JSON.stringify(result);
							const isErr =
								typeof result === 'object' &&
								result !== null &&
								'error' in result;
							sse(ctrl, 'tool_result', {
								tool_use_id: t.id,
								name: t.name,
								result: str,
								is_error: isErr
							});
							toolResults.push({
								type: 'tool_result',
								tool_use_id: t.id,
								content: str
							});
							allToolCalls.push(t);
							allToolResults.push({
								tool_use_id: t.id,
								content: str,
								is_error: isErr
							});
						}
						// Build assistant content using ContentBlockParam types
						const ac: Anthropic.ContentBlockParam[] = [];
						if (iterText) ac.push({ type: 'text', text: iterText });
						for (const t of toolBlocks) {
							ac.push({
								type: 'tool_use',
								id: t.id,
								name: t.name,
								input: t.input
							});
						}
						msgs = [
							...msgs,
							{ role: 'assistant', content: ac },
							{ role: 'user', content: toolResults }
						];
					} else {
						break;
					}
				}

				await repositories.aiChat.finalizeMessage(
					assistantMsgId,
					fullContent,
					allToolCalls.length ? JSON.stringify(allToolCalls) : null,
					allToolResults.length ? JSON.stringify(allToolResults) : null
				);

				// Auto-title session from first user message
				const sess = await repositories.aiChat.getSession(session_id);
				if (sess?.title === 'New Chat') {
					await repositories.aiChat.updateSessionTitle(
						session_id,
						message.slice(0, 60) + (message.length > 60 ? '\u2026' : '')
					);
				}

				sse(ctrl, 'done', { message_id: assistantMsgId });
			} catch (e) {
				const msg = e instanceof Error ? e.message : 'Unknown error';
				sse(ctrl, 'error', { message: msg });
				await repositories.aiChat.finalizeMessage(assistantMsgId, `Error: ${msg}`);
			}
			ctrl.close();
		}
	});

	return new Response(stream, {
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache',
			Connection: 'keep-alive'
		}
	});
};
