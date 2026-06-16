/**
 * Singleton runes store for the agentic-AI chat.
 *
 * Holds the conversation list, the active conversation's messages, and the
 * accumulating state of the in-flight turn. Wires a per-conversation SSE stream
 * (web/src/lib/agent/stream.ts) and folds each event through the pure reducer in
 * ./agentChatReducer. All network calls go through the api/agent client.
 *
 * The pure reducer + view types live in agentChatReducer.ts so they can be
 * unit-tested without the runes compiler; they are re-exported here for callers.
 */

import {
	createConversation,
	listConversations,
	listMessages,
	sendMessage,
	decide as apiDecide,
	revert as apiRevert,
	type AgentConversation,
	type AgentMessageDTO,
	type RevertResult
} from '$lib/api/agent';
import { ApiError } from '$lib/api/client';
import { openAgentStream, type StreamHandle } from '$lib/agent/stream';
import type { AgentEvent } from '$lib/agent/events';
import { applyEvent, emptyTurn, type TurnState } from './agentChatReducer';

export {
	applyEvent,
	emptyTurn,
	type TurnState,
	type ToolResultView,
	type AccessRequestInfo
} from './agentChatReducer';

export type AgentChatStatus = 'idle' | 'running' | 'awaiting' | 'error';

function createAgentChat() {
	let conversations = $state<AgentConversation[]>([]);
	let conversationId = $state<number | null>(null);
	let messages = $state<AgentMessageDTO[]>([]);
	let turn = $state<TurnState>(emptyTurn());
	let status = $state<AgentChatStatus>('idle');
	let errorText = $state('');
	let enabled = $state(true);
	let lastRevert = $state<RevertResult | null>(null);

	// Module-scoped stream handle for the active conversation (singleton resource).
	let stream: StreamHandle | null = null;

	/** Close the active SSE stream, if any. */
	function closeStream(): void {
		if (stream !== null) {
			stream.close();
			stream = null;
		}
	}

	/** Refetch the active conversation's messages (stream has no replay). */
	async function reloadMessages(): Promise<void> {
		if (conversationId === null) return;
		try {
			messages = await listMessages(conversationId);
		} catch (e) {
			// A failed reconcile is non-fatal; surface it without breaking the turn.
			errorText = e instanceof Error ? e.message : String(e);
		}
	}

	/** Fold a stream event into turn state and update status side-effects. */
	function handleEvent(ev: AgentEvent): void {
		turn = applyEvent(turn, ev);
		switch (ev.type) {
			case 'access_request':
				status = 'awaiting';
				break;
			case 'message_final':
				status = 'idle';
				// The turn is now history; reconcile from the server and reset.
				void reloadMessages().then(() => {
					turn = emptyTurn();
				});
				break;
			case 'error':
			case 'budget_exceeded':
				status = 'error';
				errorText = ev.message;
				break;
			default:
				break;
		}
	}

	/** Open a fresh stream for the given conversation, closing any prior one. */
	function openStream(id: number): void {
		closeStream();
		stream = openAgentStream(id, {
			onEvent: handleEvent,
			onOpen: () => {
				// No server-side replay — reconcile message history on every connect.
				void reloadMessages();
			}
		});
	}

	async function loadConversations(): Promise<void> {
		try {
			conversations = await listConversations();
			enabled = true;
		} catch (e) {
			if (e instanceof ApiError && e.status === 503) {
				enabled = false;
				return;
			}
			errorText = e instanceof Error ? e.message : String(e);
		}
	}

	/** Reset to a brand-new, unsent conversation. */
	function newConversation(): void {
		closeStream();
		conversationId = null;
		messages = [];
		turn = emptyTurn();
		status = 'idle';
		errorText = '';
	}

	async function selectConversation(id: number): Promise<void> {
		if (id <= 0) {
			throw new Error(`selectConversation: id must be a positive integer, got ${id}`);
		}
		conversationId = id;
		turn = emptyTurn();
		status = 'idle';
		errorText = '';
		try {
			messages = await listMessages(id);
		} catch (e) {
			if (e instanceof ApiError && e.status === 503) {
				enabled = false;
				return;
			}
			errorText = e instanceof Error ? e.message : String(e);
		}
		openStream(id);
	}

	async function send(text: string): Promise<void> {
		if (typeof text !== 'string' || text.length === 0) {
			throw new Error('send: text must be a non-empty string');
		}

		// Ensure a conversation exists.
		if (conversationId === null) {
			try {
				const conv = await createConversation();
				if (conv === null) {
					status = 'error';
					errorText = 'could not start a conversation';
					return;
				}
				conversationId = conv.id;
				conversations = [conv, ...conversations];
			} catch (e) {
				if (e instanceof ApiError && e.status === 503) {
					enabled = false;
					errorText = 'AI assistant is disabled';
					return;
				}
				status = 'error';
				errorText = e instanceof Error ? e.message : String(e);
				return;
			}
		}

		const id = conversationId;
		if (id === null) return;

		// Ensure the stream is live for this conversation.
		if (stream === null) {
			openStream(id);
		}

		// Optimistic append for snappy UX; the refetch reconciles authoritatively.
		const optimistic: AgentMessageDTO = {
			id: -Date.now(),
			conversationId: id,
			role: 'user',
			content: [{ type: 'text', text }],
			createdAt: new Date().toISOString()
		};
		messages = [...messages, optimistic];

		turn = emptyTurn();
		status = 'running';
		errorText = '';

		try {
			await sendMessage(id, text);
		} catch (e) {
			if (e instanceof ApiError && e.status === 503) {
				enabled = false;
				status = 'error';
				errorText = 'AI assistant is disabled';
				return;
			}
			if (e instanceof ApiError && e.status === 429) {
				status = 'error';
				errorText = 'rate limit reached';
				return;
			}
			status = 'error';
			errorText = e instanceof Error ? e.message : String(e);
		}
	}

	async function decide(stepId: number, allow: boolean): Promise<void> {
		if (stepId <= 0) {
			throw new Error(`decide: stepId must be a positive integer, got ${stepId}`);
		}
		// Optimistically clear the prompt; the next stream event drives the rest.
		turn = { ...turn, pendingAccess: null };
		status = 'running';
		try {
			await apiDecide(stepId, allow ? 'allow' : 'deny');
		} catch (e) {
			status = 'error';
			errorText = e instanceof Error ? e.message : String(e);
		}
	}

	async function revert(checkpointId: number): Promise<void> {
		if (checkpointId <= 0) {
			throw new Error(`revert: checkpointId must be a positive integer, got ${checkpointId}`);
		}
		try {
			lastRevert = await apiRevert(checkpointId);
			await reloadMessages();
		} catch (e) {
			status = 'error';
			errorText = e instanceof Error ? e.message : String(e);
		}
	}

	function disconnect(): void {
		closeStream();
	}

	return {
		get conversations() {
			return conversations;
		},
		get conversationId() {
			return conversationId;
		},
		get messages() {
			return messages;
		},
		get turn() {
			return turn;
		},
		get status() {
			return status;
		},
		get errorText() {
			return errorText;
		},
		get enabled() {
			return enabled;
		},
		get lastRevert() {
			return lastRevert;
		},
		loadConversations,
		newConversation,
		selectConversation,
		send,
		decide,
		revert,
		disconnect
	};
}

export const agentChat = createAgentChat();
