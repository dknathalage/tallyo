export interface ChatSubAgentRun {
	agent_id: string;
	title: string;
	tool_events: ChatToolEvent[];
	tokens: string;
	summary: string;
	queued: string[];
	steps: number;
	stopped_reason?: string;
	error?: string;
	open: boolean;
}

export interface ChatMessage {
	id: string;
	role: 'user' | 'assistant' | 'system';
	content: string;
	tool_events: ChatToolEvent[];
	pending: boolean;
	quick_reply?: { question: string; options: string[] };
	subagent_runs?: ChatSubAgentRun[];
}

export interface ChatToolEvent {
	uuid: string;
	tool_name: string;
	args: Record<string, unknown>;
	status: 'running' | 'succeeded' | 'failed' | 'pending' | 'approved' | 'rejected';
	result?: unknown;
	error?: string;
}

export interface SkillSummary {
	id: string;
	title: string;
	description: string;
	tool_count: number;
	alwaysLoaded: boolean;
	loaded: boolean;
}

export interface ChatPanelState {
	open: boolean;
	sessionId: number | null;
	sessionUuid: string | null;
	messages: ChatMessage[];
	pendingToolCalls: ChatToolEvent[];
	streaming: boolean;
	loadedSkills: string[];
	availableSkills: SkillSummary[];
}

const initial: ChatPanelState = {
	open: false,
	sessionId: null,
	sessionUuid: null,
	messages: [],
	pendingToolCalls: [],
	streaming: false,
	loadedSkills: [],
	availableSkills: []
};

let state = $state<ChatPanelState>({ ...initial, messages: [], pendingToolCalls: [] });

export function getChat(): ChatPanelState {
	return state;
}

export function setOpen(open: boolean): void {
	state.open = open;
}

export function toggleOpen(): void {
	state.open = !state.open;
}

export function setSession(sessionId: number, uuid: string): void {
	state.sessionId = sessionId;
	state.sessionUuid = uuid;
}

export function setMessages(messages: ChatMessage[]): void {
	state.messages = messages;
}

export function appendMessage(msg: ChatMessage): void {
	state.messages = [...state.messages, msg];
}

export type ChatMessagePatch = {
	[K in keyof ChatMessage]?: ChatMessage[K] | undefined;
};

export function updateLastAssistant(patch: ChatMessagePatch): void {
	const last = state.messages[state.messages.length - 1];
	if (!last || last.role !== 'assistant') return;
	const bag = { ...last } as unknown as Record<string, unknown>;
	for (const key of Object.keys(patch)) {
		const value = (patch as Record<string, unknown>)[key];
		if (value === undefined) delete bag[key];
		else bag[key] = value;
	}
	state.messages = [...state.messages.slice(0, -1), bag as unknown as ChatMessage];
}

export function setPending(calls: ChatToolEvent[]): void {
	state.pendingToolCalls = calls;
}

export function removePending(uuid: string): void {
	state.pendingToolCalls = state.pendingToolCalls.filter((c) => c.uuid !== uuid);
}

export function setStreaming(streaming: boolean): void {
	state.streaming = streaming;
}

export function clearLocal(): void {
	state.messages = [];
	state.pendingToolCalls = [];
}

export function setSkills(loaded: string[], available: SkillSummary[]): void {
	state.loadedSkills = loaded;
	state.availableSkills = available;
}

export function setLoadedSkillIds(ids: string[]): void {
	state.loadedSkills = ids;
	state.availableSkills = state.availableSkills.map((s) => ({
		...s,
		loaded: s.alwaysLoaded || ids.includes(s.id)
	}));
}
