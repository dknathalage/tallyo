import type { AiChatSession, AiChatMessage } from '$lib/types/index.js';

export interface AiChatRepository {
	getSessions(): AiChatSession[];
	getSession(id: number): AiChatSession | null;
	createSession(title?: string): number;
	updateSessionTitle(id: number, title: string): void;
	deleteSession(id: number): void;
	getSessionMessages(sessionId: number): AiChatMessage[];
	addMessage(data: {
		session_id: number;
		role: 'user' | 'assistant';
		content: string;
		tool_calls?: string | null;
		tool_results?: string | null;
		is_streaming?: number;
	}): number;
	finalizeMessage(id: number, content: string, toolCalls?: string | null, toolResults?: string | null): void;
}
