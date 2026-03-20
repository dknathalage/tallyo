import { getDb } from '../connection.js';
import { aiChatSessions, aiChatMessages } from '../drizzle-schema.js';
import { eq, asc, desc } from 'drizzle-orm';
import type { AiChatSession, AiChatMessage } from '../../types/index.js';

function mapSession(row: Record<string, unknown>): AiChatSession {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		title: row.title as string,
		created_at: (row.created_at as string) ?? '',
		updated_at: (row.updated_at as string) ?? ''
	};
}

function mapMessage(row: Record<string, unknown>): AiChatMessage {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		session_id: row.session_id as number,
		role: row.role as 'user' | 'assistant',
		content: (row.content as string) ?? '',
		tool_calls: (row.tool_calls as string) ?? null,
		tool_results: (row.tool_results as string) ?? null,
		is_streaming: row.is_streaming === true ? 1 : 0,
		created_at: (row.created_at as string) ?? ''
	};
}

export async function getSessions(): Promise<AiChatSession[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(aiChatSessions)
		.orderBy(desc(aiChatSessions.updated_at));
	return rows.map((r) => mapSession(r as Record<string, unknown>));
}

export async function getSession(id: number): Promise<AiChatSession | null> {
	const db = getDb();
	const rows = await db
		.select()
		.from(aiChatSessions)
		.where(eq(aiChatSessions.id, id));
	return rows.length > 0 ? mapSession(rows[0] as Record<string, unknown>) : null;
}

export async function createSession(title = 'New Chat'): Promise<number> {
	const db = getDb();

	const result = await db
		.insert(aiChatSessions)
		.values({
			title
		})
		.returning({ id: aiChatSessions.id });

	return result[0].id;
}

export async function updateSessionTitle(id: number, title: string): Promise<void> {
	const db = getDb();

	await db
		.update(aiChatSessions)
		.set({
			title,
			updated_at: new Date().toISOString()
		})
		.where(eq(aiChatSessions.id, id));
}

export async function deleteSession(id: number): Promise<void> {
	const db = getDb();
	await db.delete(aiChatSessions).where(eq(aiChatSessions.id, id));
}

export async function getSessionMessages(sessionId: number): Promise<AiChatMessage[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(aiChatMessages)
		.where(eq(aiChatMessages.session_id, sessionId))
		.orderBy(asc(aiChatMessages.created_at), asc(aiChatMessages.id));
	return rows.map((r) => mapMessage(r as Record<string, unknown>));
}

export async function addMessage(data: {
	session_id: number;
	role: 'user' | 'assistant';
	content: string;
	tool_calls?: string | null;
	tool_results?: string | null;
	is_streaming?: number;
}): Promise<number> {
	const db = getDb();

	const result = await db
		.insert(aiChatMessages)
		.values({
			session_id: data.session_id,
			role: data.role,
			content: data.content,
			tool_calls: data.tool_calls ?? null,
			tool_results: data.tool_results ?? null,
			is_streaming: data.is_streaming ? true : false
		})
		.returning({ id: aiChatMessages.id });

	await db
		.update(aiChatSessions)
		.set({ updated_at: new Date().toISOString() })
		.where(eq(aiChatSessions.id, data.session_id));

	return result[0].id;
}

export async function finalizeMessage(
	id: number,
	content: string,
	toolCalls?: string | null,
	toolResults?: string | null
): Promise<void> {
	const db = getDb();

	await db
		.update(aiChatMessages)
		.set({
			content,
			tool_calls: toolCalls ?? null,
			tool_results: toolResults ?? null,
			is_streaming: false
		})
		.where(eq(aiChatMessages.id, id));
}
