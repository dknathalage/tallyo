import { eq, desc, asc, gte } from 'drizzle-orm';
import { and } from 'drizzle-orm';
import { getDb, ensureMigrations } from '../connection.js';
import { aiChatSessions, aiChatMessages, aiChatToolCalls } from '../drizzle-schema.js';

export interface AiChatSession {
	id: number;
	uuid: string;
	title: string;
	loaded_skills_json: string;
	created_at: string;
	updated_at: string;
}

export interface AiChatMessage {
	id: number;
	session_id: number;
	role: string;
	content: string;
	tool_calls: string;
	tool_call_id: string;
	created_at: string;
}

export interface AiChatToolCall {
	id: number;
	uuid: string;
	session_id: number;
	message_id: number | null;
	tool_name: string;
	args_json: string;
	status: string;
	result_json: string;
	error_message: string;
	parent_tool_call_uuid: string | null;
	agent_id: string | null;
	created_at: string;
	updated_at: string;
}

function nowIso(): string {
	return new Date().toISOString();
}

export async function listSessions(): Promise<AiChatSession[]> {
	ensureMigrations();
	const rows = await getDb().select().from(aiChatSessions).orderBy(desc(aiChatSessions.updated_at)).limit(100);
	return rows as AiChatSession[];
}

export async function createSession(title: string): Promise<AiChatSession> {
	ensureMigrations();
	const safeTitle = title.slice(0, 200) || 'New chat';
	const [row] = await getDb().insert(aiChatSessions).values({ title: safeTitle }).returning();
	if (!row) throw new Error('Failed to create chat session');
	return row as AiChatSession;
}

export async function getSession(id: number): Promise<AiChatSession | null> {
	ensureMigrations();
	const rows = await getDb().select().from(aiChatSessions).where(eq(aiChatSessions.id, id)).limit(1);
	return (rows[0] as AiChatSession | undefined) ?? null;
}

export async function deleteSession(id: number): Promise<void> {
	ensureMigrations();
	await getDb().delete(aiChatSessions).where(eq(aiChatSessions.id, id));
}

export async function renameSession(id: number, title: string): Promise<void> {
	ensureMigrations();
	await getDb()
		.update(aiChatSessions)
		.set({ title: title.slice(0, 200), updated_at: nowIso() })
		.where(eq(aiChatSessions.id, id));
}

export async function listMessages(sessionId: number): Promise<AiChatMessage[]> {
	ensureMigrations();
	const rows = await getDb()
		.select()
		.from(aiChatMessages)
		.where(eq(aiChatMessages.session_id, sessionId))
		.orderBy(asc(aiChatMessages.id));
	return rows as AiChatMessage[];
}

export interface AppendMessageInput {
	session_id: number;
	role: string;
	content?: string;
	tool_calls?: string;
	tool_call_id?: string;
}

export async function appendMessage(input: AppendMessageInput): Promise<AiChatMessage> {
	ensureMigrations();
	const [row] = await getDb()
		.insert(aiChatMessages)
		.values({
			session_id: input.session_id,
			role: input.role,
			content: input.content ?? '',
			tool_calls: input.tool_calls ?? '[]',
			tool_call_id: input.tool_call_id ?? ''
		})
		.returning();
	if (!row) throw new Error('Failed to append chat message');
	await getDb()
		.update(aiChatSessions)
		.set({ updated_at: nowIso() })
		.where(eq(aiChatSessions.id, input.session_id));
	return row as AiChatMessage;
}

export async function clearMessages(sessionId: number): Promise<void> {
	ensureMigrations();
	await getDb().delete(aiChatMessages).where(eq(aiChatMessages.session_id, sessionId));
}

export async function deleteMessagesFrom(sessionId: number, fromMessageId: number): Promise<void> {
	ensureMigrations();
	await getDb()
		.delete(aiChatMessages)
		.where(and(eq(aiChatMessages.session_id, sessionId), gte(aiChatMessages.id, fromMessageId)));
}

export interface CreateToolCallInput {
	session_id: number;
	message_id: number | null;
	tool_name: string;
	args_json: string;
	status: string;
	parent_tool_call_uuid?: string | null;
	agent_id?: string | null;
}

export async function createToolCall(input: CreateToolCallInput): Promise<AiChatToolCall> {
	ensureMigrations();
	const [row] = await getDb().insert(aiChatToolCalls).values(input).returning();
	if (!row) throw new Error('Failed to create tool call');
	return row as AiChatToolCall;
}

export async function getToolCallByUuid(uuid: string): Promise<AiChatToolCall | null> {
	ensureMigrations();
	const rows = await getDb().select().from(aiChatToolCalls).where(eq(aiChatToolCalls.uuid, uuid)).limit(1);
	return (rows[0] as AiChatToolCall | undefined) ?? null;
}

export async function updateToolCall(
	uuid: string,
	patch: { status?: string; result_json?: string; error_message?: string }
): Promise<void> {
	ensureMigrations();
	await getDb()
		.update(aiChatToolCalls)
		.set({ ...patch, updated_at: nowIso() })
		.where(eq(aiChatToolCalls.uuid, uuid));
}

export async function listPendingToolCalls(sessionId: number): Promise<AiChatToolCall[]> {
	ensureMigrations();
	const rows = await getDb()
		.select()
		.from(aiChatToolCalls)
		.where(eq(aiChatToolCalls.session_id, sessionId));
	return (rows as AiChatToolCall[]).filter((r) => r.status === 'pending');
}

export async function listToolCalls(sessionId: number): Promise<AiChatToolCall[]> {
	ensureMigrations();
	const rows = await getDb()
		.select()
		.from(aiChatToolCalls)
		.where(eq(aiChatToolCalls.session_id, sessionId))
		.orderBy(asc(aiChatToolCalls.id));
	return rows as AiChatToolCall[];
}

export async function findRecentSucceededToolCall(
	sessionId: number,
	toolName: string,
	argsJson: string,
	withinMs: number
): Promise<AiChatToolCall | null> {
	ensureMigrations();
	const rows = await getDb()
		.select()
		.from(aiChatToolCalls)
		.where(eq(aiChatToolCalls.session_id, sessionId));
	const cutoff = Date.now() - withinMs;
	const all = rows as AiChatToolCall[];
	for (const r of all) {
		if (r.tool_name !== toolName) continue;
		if (r.status !== 'succeeded') continue;
		if (r.args_json !== argsJson) continue;
		const ts = r.updated_at ? new Date(r.updated_at).getTime() : 0;
		if (ts >= cutoff) return r;
	}
	return null;
}

export async function getLoadedSkills(sessionId: number): Promise<string[]> {
	ensureMigrations();
	const rows = await getDb()
		.select({ loaded: aiChatSessions.loaded_skills_json })
		.from(aiChatSessions)
		.where(eq(aiChatSessions.id, sessionId))
		.limit(1);
	const raw = rows[0]?.loaded ?? '[]';
	try {
		const parsed = JSON.parse(raw) as unknown;
		if (!Array.isArray(parsed)) return [];
		return parsed.filter((s) => typeof s === 'string').slice(0, 20);
	} catch {
		return [];
	}
}

export async function setLoadedSkills(sessionId: number, ids: string[]): Promise<void> {
	ensureMigrations();
	const safe = ids.filter((s) => typeof s === 'string').slice(0, 20);
	await getDb()
		.update(aiChatSessions)
		.set({ loaded_skills_json: JSON.stringify(safe), updated_at: nowIso() })
		.where(eq(aiChatSessions.id, sessionId));
}
