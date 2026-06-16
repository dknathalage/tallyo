import { apiGet, apiPost } from './client';

// ---- DTO types ----

export interface AgentConversation {
	id: number;
	title: string;
	createdAt: string;
	updatedAt: string;
}

export interface AgentBlock {
	type: string;
	text?: string;
	toolName?: string;
	toolUseId?: string;
	input?: unknown;
}

export interface AgentMessageDTO {
	id: number;
	conversationId: number;
	role: 'user' | 'assistant';
	content: AgentBlock[];
	createdAt: string;
}

export interface PlanStepDTO {
	tool: string;
	summary: string;
	risk: 'read' | 'risky' | 'meta';
}

export interface RevertResult {
	conflicts: { table: string; pk: number }[];
}

// ---- API functions ----

/**
 * Create a new agent conversation. The optional title is sent if provided;
 * the server may generate one automatically otherwise.
 */
export function createConversation(title?: string): Promise<AgentConversation | null> {
	const body: Record<string, string> = {};
	if (title !== undefined && title.length > 0) {
		body.title = title;
	}
	return apiPost<AgentConversation>('/api/agent/conversations', body);
}

/**
 * List all conversations for the current tenant. Returns an empty array
 * when the client returns null (e.g. 204 or 401-redirect).
 */
export async function listConversations(): Promise<AgentConversation[]> {
	const result = await apiGet<AgentConversation[]>('/api/agent/conversations');
	return result ?? [];
}

/**
 * List messages in the given conversation. Returns an empty array when
 * the client returns null.
 */
export async function listMessages(convId: number): Promise<AgentMessageDTO[]> {
	if (convId <= 0) {
		throw new Error(`listMessages: convId must be a positive integer, got ${convId}`);
	}
	const result = await apiGet<AgentMessageDTO[]>(`/api/agent/conversations/${convId}/messages`);
	return result ?? [];
}

/**
 * Send a user message to the given conversation. The server responds 202;
 * the actual reply arrives via SSE. Returns void.
 */
export async function sendMessage(convId: number, text: string): Promise<void> {
	if (convId <= 0) {
		throw new Error(`sendMessage: convId must be a positive integer, got ${convId}`);
	}
	if (text.length === 0) {
		throw new Error('sendMessage: text must be a non-empty string');
	}
	await apiPost<{ status: string; conversationId: number }>(
		`/api/agent/conversations/${convId}/messages`,
		{ text }
	);
}

/**
 * Submit an allow/deny decision for an agent plan step that is awaiting
 * human approval. Returns void.
 */
export async function decide(stepId: number, decision: 'allow' | 'deny'): Promise<void> {
	await apiPost<{ status: string; stepId: number }>(`/api/agent/steps/${stepId}/decision`, {
		decision
	});
}

/**
 * Revert the DB to the state captured in the given checkpoint. Returns the
 * revert result (which may include conflict rows), or null if the client
 * returned null (401 redirect / 204).
 */
export function revert(checkpointId: number): Promise<RevertResult | null> {
	return apiPost<RevertResult>(`/api/agent/checkpoints/${checkpointId}/revert`, {});
}
