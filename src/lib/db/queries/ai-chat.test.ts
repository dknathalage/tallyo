import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn()
}));

import {
	getSessions,
	getSession,
	createSession,
	updateSessionTitle,
	deleteSession,
	getSessionMessages,
	addMessage,
	finalizeMessage
} from './ai-chat.js';
import { query, execute } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getSessions', () => {
	it('returns all sessions ordered by updated_at DESC', () => {
		const sessions = [
			{ id: 2, title: 'Second', created_at: '', updated_at: '2026-01-02' },
			{ id: 1, title: 'First', created_at: '', updated_at: '2026-01-01' }
		];
		mockQuery.mockReturnValue(sessions);

		const result = getSessions();

		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM ai_chat_sessions ORDER BY updated_at DESC');
		expect(result).toEqual(sessions);
	});

	it('returns empty array when no sessions exist', () => {
		mockQuery.mockReturnValue([]);

		const result = getSessions();

		expect(result).toEqual([]);
	});
});

describe('getSession', () => {
	it('returns session when found', () => {
		const session = { id: 1, title: 'Test', created_at: '', updated_at: '' };
		mockQuery.mockReturnValue([session]);

		const result = getSession(1);

		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM ai_chat_sessions WHERE id = ?', [1]);
		expect(result).toEqual(session);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		const result = getSession(999);

		expect(result).toBeNull();
	});
});

describe('createSession', () => {
	it('inserts session and returns id', () => {
		mockQuery.mockReturnValue([{ id: 42 }]);

		const id = createSession('My Chat');

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO ai_chat_sessions (title) VALUES (?)',
			['My Chat']
		);
		expect(mockQuery).toHaveBeenCalledWith('SELECT last_insert_rowid() as id');
		expect(id).toBe(42);
	});

	it('defaults title to New Chat', () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		createSession();

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO ai_chat_sessions (title) VALUES (?)',
			['New Chat']
		);
	});
});

describe('updateSessionTitle', () => {
	it('updates title and updated_at', () => {
		updateSessionTitle(5, 'Renamed');

		expect(mockExecute).toHaveBeenCalledWith(
			"UPDATE ai_chat_sessions SET title = ?, updated_at = datetime('now') WHERE id = ?",
			['Renamed', 5]
		);
	});
});

describe('deleteSession', () => {
	it('deletes session by id', () => {
		deleteSession(3);

		expect(mockExecute).toHaveBeenCalledWith(
			'DELETE FROM ai_chat_sessions WHERE id = ?',
			[3]
		);
	});

	it('propagates errors from execute', () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('DELETE failed');
		});

		expect(() => deleteSession(3)).toThrow('DELETE failed');
	});
});

describe('getSessionMessages', () => {
	it('returns messages ordered by created_at ASC, id ASC', () => {
		const messages = [
			{ id: 1, session_id: 10, role: 'user', content: 'Hi', created_at: '2026-01-01' },
			{ id: 2, session_id: 10, role: 'assistant', content: 'Hello', created_at: '2026-01-01' }
		];
		mockQuery.mockReturnValue(messages);

		const result = getSessionMessages(10);

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT * FROM ai_chat_messages WHERE session_id = ? ORDER BY created_at ASC, id ASC',
			[10]
		);
		expect(result).toEqual(messages);
	});

	it('returns empty array when no messages', () => {
		mockQuery.mockReturnValue([]);

		const result = getSessionMessages(999);

		expect(result).toEqual([]);
	});
});

describe('addMessage', () => {
	it('inserts message, updates session, and returns id', () => {
		mockQuery.mockReturnValue([{ id: 7 }]);

		const id = addMessage({
			session_id: 10,
			role: 'user',
			content: 'Hello'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO ai_chat_messages (session_id, role, content, tool_calls, tool_results, is_streaming) VALUES (?, ?, ?, ?, ?, ?)',
			[10, 'user', 'Hello', null, null, 0]
		);
		expect(mockQuery).toHaveBeenCalledWith('SELECT last_insert_rowid() as id');
		expect(mockExecute).toHaveBeenCalledWith(
			"UPDATE ai_chat_sessions SET updated_at = datetime('now') WHERE id = ?",
			[10]
		);
		expect(id).toBe(7);
	});

	it('passes optional tool_calls, tool_results, and is_streaming', () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		addMessage({
			session_id: 5,
			role: 'assistant',
			content: 'response',
			tool_calls: '{"tool":"search"}',
			tool_results: '{"result":"found"}',
			is_streaming: 1
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO ai_chat_messages'),
			[5, 'assistant', 'response', '{"tool":"search"}', '{"result":"found"}', 1]
		);
	});

	it('defaults tool_calls and tool_results to null and is_streaming to 0', () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		addMessage({
			session_id: 1,
			role: 'user',
			content: 'test'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO ai_chat_messages'),
			[1, 'user', 'test', null, null, 0]
		);
	});
});

describe('finalizeMessage', () => {
	it('updates content and sets is_streaming to 0', () => {
		finalizeMessage(3, 'Final content');

		expect(mockExecute).toHaveBeenCalledWith(
			'UPDATE ai_chat_messages SET content = ?, tool_calls = ?, tool_results = ?, is_streaming = 0 WHERE id = ?',
			['Final content', null, null, 3]
		);
	});

	it('passes tool_calls and tool_results when provided', () => {
		finalizeMessage(3, 'Done', '{"calls":"data"}', '{"results":"data"}');

		expect(mockExecute).toHaveBeenCalledWith(
			'UPDATE ai_chat_messages SET content = ?, tool_calls = ?, tool_results = ?, is_streaming = 0 WHERE id = ?',
			['Done', '{"calls":"data"}', '{"results":"data"}', 3]
		);
	});

	it('defaults null tool_calls and tool_results to null', () => {
		finalizeMessage(1, 'content', null, null);

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE ai_chat_messages'),
			['content', null, null, 1]
		);
	});
});
