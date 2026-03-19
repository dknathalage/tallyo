import type { AiChatSession, AiChatMessage } from '$lib/types/index.js';

export interface AiChatRepository {
	getSessions(): Promise<AiChatSession[]>;
	getSession(id: number): Promise<AiChatSession | null>;
	createSession(title?: string): Promise<number>;
	updateSessionTitle(id: number, title: string): Promise<void>;
	deleteSession(id: number): Promise<void>;
	getSessionMessages(sessionId: number): Promise<AiChatMessage[]>;
	addMessage(data: {
		session_id: number;
		role: 'user' | 'assistant';
		content: string;
		tool_calls?: string | null;
		tool_results?: string | null;
		is_streaming?: number;
	}): Promise<number>;
	finalizeMessage(id: number, content: string, toolCalls?: string | null, toolResults?: string | null): Promise<void>;
}
