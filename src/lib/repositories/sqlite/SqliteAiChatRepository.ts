import * as q from '$lib/db/queries/ai-chat.js';
import type { AiChatRepository } from '../interfaces/AiChatRepository.js';

export class SqliteAiChatRepository implements AiChatRepository {
	listSessions = q.listSessions;
	createSession = q.createSession;
	getSession = q.getSession;
	deleteSession = q.deleteSession;
	renameSession = q.renameSession;
	listMessages = q.listMessages;
	appendMessage = q.appendMessage;
	clearMessages = q.clearMessages;
	deleteMessagesFrom = q.deleteMessagesFrom;
	createToolCall = q.createToolCall;
	getToolCallByUuid = q.getToolCallByUuid;
	updateToolCall = q.updateToolCall;
	listPendingToolCalls = q.listPendingToolCalls;
	listToolCalls = q.listToolCalls;
	findRecentSucceededToolCall = q.findRecentSucceededToolCall;
	getLoadedSkills = q.getLoadedSkills;
	setLoadedSkills = q.setLoadedSkills;
}
