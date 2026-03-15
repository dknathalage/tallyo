import { query, execute } from '../connection.js';
import type { AiChatSession, AiChatMessage } from '../../types/index.js';

export function getSessions(): AiChatSession[] {
	return query<AiChatSession>('SELECT * FROM ai_chat_sessions ORDER BY updated_at DESC');
}

export function getSession(id: number): AiChatSession | null {
	const rows = query<AiChatSession>('SELECT * FROM ai_chat_sessions WHERE id = ?', [id]);
	return rows[0] ?? null;
}

export function createSession(title = 'New Chat'): number {
	execute('INSERT INTO ai_chat_sessions (title) VALUES (?)', [title]);
	const result = query<{ id: number }>('SELECT last_insert_rowid() as id');
	return result[0].id;
}

export function updateSessionTitle(id: number, title: string): void {
	execute("UPDATE ai_chat_sessions SET title = ?, updated_at = datetime('now') WHERE id = ?", [title, id]);
}

export function deleteSession(id: number): void {
	execute('DELETE FROM ai_chat_sessions WHERE id = ?', [id]);
}

export function getSessionMessages(sessionId: number): AiChatMessage[] {
	return query<AiChatMessage>(
		'SELECT * FROM ai_chat_messages WHERE session_id = ? ORDER BY created_at ASC, id ASC',
		[sessionId]
	);
}

export function addMessage(data: {
	session_id: number;
	role: 'user' | 'assistant';
	content: string;
	tool_calls?: string | null;
	tool_results?: string | null;
	is_streaming?: number;
}): number {
	execute(
		'INSERT INTO ai_chat_messages (session_id, role, content, tool_calls, tool_results, is_streaming) VALUES (?, ?, ?, ?, ?, ?)',
		[
			data.session_id,
			data.role,
			data.content,
			data.tool_calls ?? null,
			data.tool_results ?? null,
			data.is_streaming ?? 0
		]
	);
	const result = query<{ id: number }>('SELECT last_insert_rowid() as id');
	execute("UPDATE ai_chat_sessions SET updated_at = datetime('now') WHERE id = ?", [data.session_id]);
	return result[0].id;
}

export function finalizeMessage(
	id: number,
	content: string,
	toolCalls?: string | null,
	toolResults?: string | null
): void {
	execute(
		'UPDATE ai_chat_messages SET content = ?, tool_calls = ?, tool_results = ?, is_streaming = 0 WHERE id = ?',
		[content, toolCalls ?? null, toolResults ?? null, id]
	);
}
