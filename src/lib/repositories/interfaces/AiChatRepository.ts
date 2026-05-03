import type {
	AiChatSession,
	AiChatMessage,
	AiChatToolCall,
	AppendMessageInput,
	CreateToolCallInput
} from '$lib/db/queries/ai-chat.js';

export interface AiChatRepository {
	listSessions(): Promise<AiChatSession[]>;
	createSession(title: string): Promise<AiChatSession>;
	getSession(id: number): Promise<AiChatSession | null>;
	deleteSession(id: number): Promise<void>;
	renameSession(id: number, title: string): Promise<void>;

	listMessages(sessionId: number): Promise<AiChatMessage[]>;
	appendMessage(input: AppendMessageInput): Promise<AiChatMessage>;
	clearMessages(sessionId: number): Promise<void>;
	deleteMessagesFrom(sessionId: number, fromMessageId: number): Promise<void>;

	createToolCall(input: CreateToolCallInput): Promise<AiChatToolCall>;
	getToolCallByUuid(uuid: string): Promise<AiChatToolCall | null>;
	updateToolCall(
		uuid: string,
		patch: { status?: string; result_json?: string; error_message?: string }
	): Promise<void>;
	listPendingToolCalls(sessionId: number): Promise<AiChatToolCall[]>;
	listToolCalls(sessionId: number): Promise<AiChatToolCall[]>;
	findRecentSucceededToolCall(
		sessionId: number,
		toolName: string,
		argsJson: string,
		withinMs: number
	): Promise<AiChatToolCall | null>;

	getLoadedSkills(sessionId: number): Promise<string[]>;
	setLoadedSkills(sessionId: number, ids: string[]): Promise<void>;
}

export type { AiChatSession, AiChatMessage, AiChatToolCall };
