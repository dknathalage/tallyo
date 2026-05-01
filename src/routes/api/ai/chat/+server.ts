import { error } from '@sveltejs/kit';
import Anthropic from '@anthropic-ai/sdk';
import { repositories } from '$lib/repositories/index.js';
import { AI_TOOLS, SYSTEM_PROMPT, executeTool } from '$lib/server/ai-tools.js';
import type { RequestHandler } from './$types.js';

const MAX_ITER = 10;
const MAX_TITLE_LEN = 60;

const encoder = new TextEncoder();

function sse(ctrl: ReadableStreamDefaultController, event: string, data: unknown): void {
	ctrl.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
}

function readApiKey(metadata: string | null | undefined): string | undefined {
	try {
		const m = JSON.parse(metadata ?? '{}') as Record<string, string>;
		return m['anthropic_api_key'];
	} catch {
		return undefined;
	}
}

interface ToolBlock {
	id: string;
	name: string;
	input: Record<string, unknown>;
}

interface IterationStreamResult {
	iterText: string;
	toolBlocks: ToolBlock[];
}

async function consumeStream(
	msgStream: ReturnType<Anthropic['messages']['stream']>,
	ctrl: ReadableStreamDefaultController,
	onText: (delta: string) => void
): Promise<IterationStreamResult> {
	let iterText = '';
	const toolBlocks: ToolBlock[] = [];
	let curTool: { id: string; name: string; inputJson: string } | null = null;

	for await (const ev of msgStream) {
		if (ev.type === 'content_block_start' && ev.content_block.type === 'tool_use') {
			curTool = { id: ev.content_block.id, name: ev.content_block.name, inputJson: '' };
			sse(ctrl, 'tool_start', { id: ev.content_block.id, name: ev.content_block.name });
		} else if (ev.type === 'content_block_delta') {
			if (ev.delta.type === 'text_delta') {
				iterText += ev.delta.text;
				onText(ev.delta.text);
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
	return { iterText, toolBlocks };
}

interface ToolExecutionOutcome {
	toolResults: Anthropic.ToolResultBlockParam[];
	allCalls: unknown[];
	allResults: unknown[];
}

async function runTools(
	toolBlocks: ToolBlock[],
	ctrl: ReadableStreamDefaultController
): Promise<ToolExecutionOutcome> {
	const toolResults: Anthropic.ToolResultBlockParam[] = [];
	const allCalls: unknown[] = [];
	const allResults: unknown[] = [];
	for (const t of toolBlocks) {
		const result = await executeTool(t.name, t.input);
		const str = JSON.stringify(result);
		const isErr = typeof result === 'object' && result !== null && 'error' in result;
		sse(ctrl, 'tool_result', {
			tool_use_id: t.id,
			name: t.name,
			result: str,
			is_error: isErr
		});
		toolResults.push({ type: 'tool_result', tool_use_id: t.id, content: str });
		allCalls.push(t);
		allResults.push({ tool_use_id: t.id, content: str, is_error: isErr });
	}
	return { toolResults, allCalls, allResults };
}

function buildAssistantContent(
	iterText: string,
	toolBlocks: ToolBlock[]
): Anthropic.ContentBlockParam[] {
	const ac: Anthropic.ContentBlockParam[] = [];
	if (iterText) ac.push({ type: 'text', text: iterText });
	for (const t of toolBlocks) {
		ac.push({ type: 'tool_use', id: t.id, name: t.name, input: t.input });
	}
	return ac;
}

interface RunState {
	fullContent: string;
	allToolCalls: unknown[];
	allToolResults: unknown[];
}

async function runConversation(
	anthropic: Anthropic,
	initialMsgs: Anthropic.MessageParam[],
	ctrl: ReadableStreamDefaultController
): Promise<RunState> {
	let msgs = initialMsgs;
	const state: RunState = { fullContent: '', allToolCalls: [], allToolResults: [] };

	for (let iter = 0; iter < MAX_ITER; iter++) {
		const msgStream = anthropic.messages.stream({
			model: 'claude-opus-4-5',
			max_tokens: 4096,
			system: SYSTEM_PROMPT,
			tools: AI_TOOLS,
			messages: msgs
		});
		const { iterText, toolBlocks } = await consumeStream(msgStream, ctrl, (d) => {
			state.fullContent += d;
		});

		const final = await msgStream.finalMessage();
		if (final.stop_reason !== 'tool_use' || toolBlocks.length === 0) break;

		const { toolResults, allCalls, allResults } = await runTools(toolBlocks, ctrl);
		state.allToolCalls.push(...allCalls);
		state.allToolResults.push(...allResults);

		msgs = [
			...msgs,
			{ role: 'assistant', content: buildAssistantContent(iterText, toolBlocks) },
			{ role: 'user', content: toolResults }
		];
	}
	return state;
}

async function maybeAutoTitle(session_id: number, message: string): Promise<void> {
	const sess = await repositories.aiChat.getSession(session_id);
	if (sess?.title === 'New Chat') {
		const title = message.slice(0, MAX_TITLE_LEN) + (message.length > MAX_TITLE_LEN ? '…' : '');
		await repositories.aiChat.updateSessionTitle(session_id, title);
	}
}

interface StartStreamingArgs {
	ctrl: ReadableStreamDefaultController;
	apiKey: string | undefined;
	session_id: number;
	message: string;
	history: Awaited<ReturnType<typeof repositories.aiChat.getSessionMessages>>;
}

async function startStreaming(args: StartStreamingArgs): Promise<void> {
	const { ctrl, apiKey, session_id, message, history } = args;
	if (!apiKey) {
		sse(ctrl, 'error', {
			message: 'No Anthropic API key configured. Please add it in Settings → AI Assistant.'
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

	const initialMsgs: Anthropic.MessageParam[] = history.map((m) => ({
		role: m.role,
		content: m.content
	}));

	try {
		const state = await runConversation(anthropic, initialMsgs, ctrl);
		await repositories.aiChat.finalizeMessage(
			assistantMsgId,
			state.fullContent,
			state.allToolCalls.length ? JSON.stringify(state.allToolCalls) : null,
			state.allToolResults.length ? JSON.stringify(state.allToolResults) : null
		);
		await maybeAutoTitle(session_id, message);
		sse(ctrl, 'done', { message_id: assistantMsgId });
	} catch (e) {
		const msg = e instanceof Error ? e.message : 'Unknown error';
		sse(ctrl, 'error', { message: msg });
		await repositories.aiChat.finalizeMessage(assistantMsgId, `Error: ${msg}`);
	}
	ctrl.close();
}

export const POST: RequestHandler = async ({ request }) => {
	const body = (await request.json()) as { session_id?: number; message?: string };
	const { session_id, message } = body;
	if (!session_id || !message) throw error(400, 'session_id and message required');

	const profile = await repositories.businessProfile.getBusinessProfile();
	const apiKey = readApiKey(profile?.metadata);

	await repositories.aiChat.addMessage({ session_id, role: 'user', content: message });
	const history = await repositories.aiChat.getSessionMessages(session_id);

	const stream = new ReadableStream({
		start(ctrl) {
			void startStreaming({ ctrl, apiKey, session_id, message, history });
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
