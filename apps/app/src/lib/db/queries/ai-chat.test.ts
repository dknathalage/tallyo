import { describe, it, expect, vi, beforeEach } from 'vitest';

// Create a mock db object with chainable Drizzle-like methods
function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'having', 'offset'];
	for (const m of methods) {
		chain[m] = vi.fn().mockReturnValue(chain);
	}
	// Default: resolve to empty array (for selects)
	chain.then = (resolve: any) => resolve([]);
	chain[Symbol.iterator] = function* () {};
	// For transaction support
	chain.transaction = vi.fn(async (fn: any) => fn(chain));
	return chain;
}

const mockDb = createMockDb();

vi.mock('../connection.js', () => ({
	getDb: vi.fn(() => mockDb)
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

beforeEach(() => {
	vi.clearAllMocks();
	// Reset chain resolution to empty array
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'having', 'offset']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getSessions', () => {
	it('is an async function that returns a promise', () => {
		expect(getSessions()).toBeInstanceOf(Promise);
	});

	it('returns sessions array', async () => {
		const sessions = [
			{ id: 2, title: 'Second', created_at: '', updated_at: '2026-01-02' },
			{ id: 1, title: 'First', created_at: '', updated_at: '2026-01-01' }
		];
		mockDb.then = (resolve: any) => resolve(sessions);

		const result = await getSessions();
		expect(result).toEqual(sessions);
	});

	it('returns empty array when no sessions exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getSessions();
		expect(result).toEqual([]);
	});
});

describe('getSession', () => {
	it('is an async function', () => {
		expect(getSession(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getSession(999);
		expect(result).toBeNull();
	});
});

describe('createSession', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 42 }]);
		expect(createSession('My Chat')).toBeInstanceOf(Promise);
	});
});

describe('updateSessionTitle', () => {
	it('is an async function', () => {
		expect(updateSessionTitle(5, 'Renamed')).toBeInstanceOf(Promise);
	});
});

describe('deleteSession', () => {
	it('is an async function', () => {
		expect(deleteSession(3)).toBeInstanceOf(Promise);
	});

	it('propagates errors', async () => {
		mockDb.where.mockRejectedValueOnce(new Error('DELETE failed'));
		await expect(deleteSession(3)).rejects.toThrow('DELETE failed');
	});
});

describe('getSessionMessages', () => {
	it('is an async function', () => {
		expect(getSessionMessages(10)).toBeInstanceOf(Promise);
	});

	it('returns empty array when no messages', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getSessionMessages(999);
		expect(result).toEqual([]);
	});
});

describe('addMessage', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 7 }]);
		expect(addMessage({
			session_id: 10,
			role: 'user',
			content: 'Hello'
		})).toBeInstanceOf(Promise);
	});
});

describe('finalizeMessage', () => {
	it('is an async function', () => {
		expect(finalizeMessage(3, 'Final content')).toBeInstanceOf(Promise);
	});
});
