import {
	getSessions,
	getSession,
	createSession,
	updateSessionTitle,
	deleteSession,
	getSessionMessages,
	addMessage,
	finalizeMessage
} from '$lib/db/queries/ai-chat.js';
import type { AiChatRepository } from '../interfaces/AiChatRepository.js';
import type { AiChatSession, AiChatMessage } from '$lib/types/index.js';

export class SqliteAiChatRepository implements AiChatRepository {
	getSessions(): AiChatSession[] {
		return getSessions();
	}

	getSession(id: number): AiChatSession | null {
		return getSession(id);
	}

	createSession(title?: string): number {
		return createSession(title);
	}

	updateSessionTitle(id: number, title: string): void {
		return updateSessionTitle(id, title);
	}

	deleteSession(id: number): void {
		return deleteSession(id);
	}

	getSessionMessages(sessionId: number): AiChatMessage[] {
		return getSessionMessages(sessionId);
	}

	addMessage(data: {
		session_id: number;
		role: 'user' | 'assistant';
		content: string;
		tool_calls?: string | null;
		tool_results?: string | null;
		is_streaming?: number;
	}): number {
		return addMessage(data);
	}

	finalizeMessage(id: number, content: string, toolCalls?: string | null, toolResults?: string | null): void {
		return finalizeMessage(id, content, toolCalls, toolResults);
	}
}
