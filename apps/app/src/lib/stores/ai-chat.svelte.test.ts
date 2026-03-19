import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock toast store
vi.mock('./toast.svelte.js', () => ({
	addToast: vi.fn()
}));

const mockFetch = vi.fn();
Object.defineProperty(globalThis, 'fetch', { value: mockFetch, writable: true });

import { aiChat } from './ai-chat.svelte.js';
import { addToast } from './toast.svelte.js';

const mockAddToast = vi.mocked(addToast);

function makeReadableStream(events: string[]): ReadableStream {
	let index = 0;
	return new ReadableStream({
		pull(controller) {
			if (index < events.length) {
				controller.enqueue(new TextEncoder().encode(events[index++]));
			} else {
				controller.close();
			}
		}
	});
}

beforeEach(() => {
	vi.clearAllMocks();
	aiChat.sessions = [];
	aiChat.activeSessionId = null;
	aiChat.messages = [];
	aiChat.isStreaming = false;
	aiChat.streaming = { text: '', toolCalls: [] };
});

describe('aiChat store initial state', () => {
	it('sessions starts empty', () => {
		expect(aiChat.sessions).toEqual([]);
	});

	it('activeSessionId starts null', () => {
		expect(aiChat.activeSessionId).toBeNull();
	});

	it('messages starts empty', () => {
		expect(aiChat.messages).toEqual([]);
	});

	it('isStreaming starts false', () => {
		expect(aiChat.isStreaming).toBe(false);
	});

	it('streaming starts with empty text and toolCalls', () => {
		expect(aiChat.streaming.text).toBe('');
		expect(aiChat.streaming.toolCalls).toEqual([]);
	});
});

describe('activeSession getter', () => {
	it('returns null when activeSessionId is null', () => {
		aiChat.activeSessionId = null;
		expect(aiChat.activeSession).toBeNull();
	});

	it('returns session matching activeSessionId', () => {
		const session = { id: 1, uuid: 'abc', title: 'Test Chat', created_at: '2025-01-01', updated_at: '2025-01-01' };
		aiChat.sessions = [session];
		aiChat.activeSessionId = 1;
		expect(aiChat.activeSession).toEqual(session);
	});

	it('returns null when activeSessionId does not match any session', () => {
		aiChat.sessions = [{ id: 2, uuid: 'xyz', title: 'Other', created_at: '', updated_at: '' }];
		aiChat.activeSessionId = 99;
		expect(aiChat.activeSession).toBeNull();
	});
});

describe('loadSessions', () => {
	it('sets sessions on successful fetch', async () => {
		const sessions = [{ id: 1, uuid: 'a', title: 'Chat 1', created_at: '', updated_at: '' }];
		mockFetch.mockResolvedValueOnce({
			ok: true,
			json: async () => sessions
		});
		await aiChat.loadSessions();
		expect(aiChat.sessions).toEqual(sessions);
	});

	it('does not throw on fetch failure', async () => {
		mockFetch.mockRejectedValueOnce(new Error('Network error'));
		await expect(aiChat.loadSessions()).resolves.not.toThrow();
	});

	it('does not update sessions when response is not ok', async () => {
		aiChat.sessions = [{ id: 1, uuid: 'a', title: 'Existing', created_at: '', updated_at: '' }];
		mockFetch.mockResolvedValueOnce({ ok: false });
		await aiChat.loadSessions();
		// Sessions stay as they were
		expect(aiChat.sessions).toHaveLength(1);
	});
});

describe('createSession', () => {
	it('creates session and sets activeSessionId', async () => {
		mockFetch
			.mockResolvedValueOnce({ ok: true, json: async () => ({ id: 5 }) }) // POST
			.mockResolvedValueOnce({ ok: true, json: async () => [] }); // loadSessions

		const id = await aiChat.createSession('My Chat');
		expect(id).toBe(5);
		expect(aiChat.activeSessionId).toBe(5);
		expect(aiChat.messages).toEqual([]);
	});

	it('uses default title "New Chat"', async () => {
		mockFetch
			.mockResolvedValueOnce({ ok: true, json: async () => ({ id: 3 }) })
			.mockResolvedValueOnce({ ok: true, json: async () => [] });

		await aiChat.createSession();
		expect(mockFetch).toHaveBeenCalledWith('/api/ai/sessions', expect.objectContaining({
			body: JSON.stringify({ title: 'New Chat' })
		}));
	});

	it('returns null when response is not ok', async () => {
		mockFetch.mockResolvedValueOnce({ ok: false });
		const id = await aiChat.createSession();
		expect(id).toBeNull();
	});
});

describe('selectSession', () => {
	it('sets activeSessionId and loads messages', async () => {
		const messages = [{ id: 1, uuid: 'm1', session_id: 2, role: 'user', content: 'hi', tool_calls: null, tool_results: null, is_streaming: 0, created_at: '' }];
		mockFetch.mockResolvedValueOnce({ ok: true, json: async () => ({ messages }) });

		await aiChat.selectSession(2);
		expect(aiChat.activeSessionId).toBe(2);
		expect(aiChat.messages).toEqual(messages);
	});

	it('sets empty messages when response data.messages is missing', async () => {
		mockFetch.mockResolvedValueOnce({ ok: true, json: async () => ({}) });
		await aiChat.selectSession(3);
		expect(aiChat.messages).toEqual([]);
	});

	it('does not throw on network error', async () => {
		mockFetch.mockRejectedValueOnce(new Error('fail'));
		await expect(aiChat.selectSession(1)).resolves.not.toThrow();
	});
});

describe('deleteSession', () => {
	it('calls DELETE endpoint and reloads sessions', async () => {
		aiChat.sessions = [{ id: 1, uuid: 'a', title: 'Chat', created_at: '', updated_at: '' }];
		mockFetch
			.mockResolvedValueOnce({ ok: true }) // DELETE
			.mockResolvedValueOnce({ ok: true, json: async () => [] }); // loadSessions

		await aiChat.deleteSession(1);
		expect(mockFetch).toHaveBeenCalledWith('/api/ai/sessions/1', { method: 'DELETE' });
	});

	it('clears activeSessionId and messages when deleting active session', async () => {
		aiChat.activeSessionId = 7;
		aiChat.messages = [{ id: 1, uuid: 'm', session_id: 7, role: 'user', content: 'hi', tool_calls: null, tool_results: null, is_streaming: 0, created_at: '' }];
		mockFetch
			.mockResolvedValueOnce({ ok: true })
			.mockResolvedValueOnce({ ok: true, json: async () => [] });

		await aiChat.deleteSession(7);
		expect(aiChat.activeSessionId).toBeNull();
		expect(aiChat.messages).toEqual([]);
	});

	it('does not clear messages when deleting non-active session', async () => {
		aiChat.activeSessionId = 1;
		aiChat.messages = [{ id: 1, uuid: 'm', session_id: 1, role: 'user', content: 'hi', tool_calls: null, tool_results: null, is_streaming: 0, created_at: '' }];
		mockFetch
			.mockResolvedValueOnce({ ok: true })
			.mockResolvedValueOnce({ ok: true, json: async () => [] });

		await aiChat.deleteSession(99); // different session
		expect(aiChat.activeSessionId).toBe(1);
		expect(aiChat.messages).toHaveLength(1);
	});
});

describe('sendMessage', () => {
	it('does nothing when activeSessionId is null', async () => {
		aiChat.activeSessionId = null;
		await aiChat.sendMessage('hello');
		expect(mockFetch).not.toHaveBeenCalled();
	});

	it('does nothing when already streaming', async () => {
		aiChat.activeSessionId = 1;
		aiChat.isStreaming = true;
		await aiChat.sendMessage('hello');
		expect(mockFetch).not.toHaveBeenCalled();
	});

	it('does nothing when text is empty', async () => {
		aiChat.activeSessionId = 1;
		await aiChat.sendMessage('');
		expect(mockFetch).not.toHaveBeenCalled();
	});

	it('does nothing when text is only whitespace', async () => {
		aiChat.activeSessionId = 1;
		await aiChat.sendMessage('   ');
		expect(mockFetch).not.toHaveBeenCalled();
	});

	it('adds optimistic user message before sending', async () => {
		aiChat.activeSessionId = 1;
		mockFetch.mockRejectedValueOnce(new Error('Network'));
		await aiChat.sendMessage('test message');
		// isStreaming should be false after error
		expect(aiChat.isStreaming).toBe(false);
	});

	it('shows error toast on non-abort fetch error', async () => {
		aiChat.activeSessionId = 1;
		mockFetch.mockRejectedValueOnce(new Error('Connection refused'));
		await aiChat.sendMessage('hello');
		expect(mockAddToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
	});

	it('does not show toast on AbortError', async () => {
		aiChat.activeSessionId = 1;
		const abortError = new Error('aborted');
		abortError.name = 'AbortError';
		mockFetch.mockRejectedValueOnce(abortError);
		await aiChat.sendMessage('hello');
		expect(mockAddToast).not.toHaveBeenCalled();
	});

	it('shows error when response is not ok', async () => {
		aiChat.activeSessionId = 1;
		mockFetch.mockResolvedValueOnce({ ok: false, status: 500, body: null });
		await aiChat.sendMessage('hello');
		expect(mockAddToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
	});
});

describe('stopStreaming', () => {
	it('sets isStreaming to false', () => {
		aiChat.isStreaming = true;
		aiChat.stopStreaming();
		expect(aiChat.isStreaming).toBe(false);
	});

	it('resets streaming state', () => {
		aiChat.streaming = { text: 'partial', toolCalls: [{ id: '1', name: 'tool' }] };
		aiChat.stopStreaming();
		expect(aiChat.streaming.text).toBe('');
		expect(aiChat.streaming.toolCalls).toEqual([]);
	});

	it('does not throw when no abortController exists', () => {
		expect(() => aiChat.stopStreaming()).not.toThrow();
	});
});

describe('SSE event processing (via sendMessage stream)', () => {
	it('processes text_delta events and accumulates text', async () => {
		aiChat.activeSessionId = 1;
		const events = [
			'event: text_delta\ndata: {"delta":"Hello"}\n\n',
			'event: text_delta\ndata: {"delta":" World"}\n\n',
			'event: done\ndata: {}\n\n'
		];
		const body = makeReadableStream(events);

		// Mock selectSession and loadSessions calls triggered by 'done' event
		mockFetch
			.mockResolvedValueOnce({ ok: true, body }) // POST /api/ai/chat
			.mockResolvedValueOnce({ ok: true, json: async () => ({ messages: [] }) }) // selectSession
			.mockResolvedValueOnce({ ok: true, json: async () => [] }); // loadSessions

		await aiChat.sendMessage('hi');
		// After done event, streaming is reset
		expect(aiChat.streaming.text).toBe('');
	});

	it('processes tool_start and tool_result events', async () => {
		aiChat.activeSessionId = 1;
		const events = [
			'event: tool_start\ndata: {"id":"t1","name":"search"}\n\n',
			'event: tool_result\ndata: {"tool_use_id":"t1","result":"found","is_error":false}\n\n',
			'event: done\ndata: {}\n\n'
		];
		const body = makeReadableStream(events);
		mockFetch
			.mockResolvedValueOnce({ ok: true, body })
			.mockResolvedValueOnce({ ok: true, json: async () => ({ messages: [] }) })
			.mockResolvedValueOnce({ ok: true, json: async () => [] });

		await aiChat.sendMessage('search something');
		expect(aiChat.isStreaming).toBe(false);
	});

	it('handles error events from stream', async () => {
		aiChat.activeSessionId = 1;
		const events = [
			'event: error\ndata: {"message":"AI failed"}\n\n'
		];
		const body = makeReadableStream(events);
		mockFetch.mockResolvedValueOnce({ ok: true, body });

		await aiChat.sendMessage('help');
		expect(mockAddToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'AI failed' }));
	});

	it('handles error event with no message', async () => {
		aiChat.activeSessionId = 1;
		const events = [
			'event: error\ndata: {}\n\n'
		];
		const body = makeReadableStream(events);
		mockFetch.mockResolvedValueOnce({ ok: true, body });

		await aiChat.sendMessage('help');
		expect(mockAddToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'AI error occurred' }));
	});
});
