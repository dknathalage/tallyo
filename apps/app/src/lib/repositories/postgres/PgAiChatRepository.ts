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

export class PgAiChatRepository implements AiChatRepository {
	async getSessions(): Promise<AiChatSession[]> {
		return await getSessions();
	}

	async getSession(id: number): Promise<AiChatSession | null> {
		return await getSession(id);
	}

	async createSession(title?: string): Promise<number> {
		return await createSession(title);
	}

	async updateSessionTitle(id: number, title: string): Promise<void> {
		return await updateSessionTitle(id, title);
	}

	async deleteSession(id: number): Promise<void> {
		return await deleteSession(id);
	}

	async getSessionMessages(sessionId: number): Promise<AiChatMessage[]> {
		return await getSessionMessages(sessionId);
	}

	async addMessage(data: {
		session_id: number;
		role: 'user' | 'assistant';
		content: string;
		tool_calls?: string | null;
		tool_results?: string | null;
		is_streaming?: number;
	}): Promise<number> {
		return await addMessage(data);
	}

	async finalizeMessage(id: number, content: string, toolCalls?: string | null, toolResults?: string | null): Promise<void> {
		return await finalizeMessage(id, content, toolCalls, toolResults);
	}
}
