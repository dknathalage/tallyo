import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ApiError } from './client';

// Mock the client module before importing agent.ts
vi.mock('./client', async (importOriginal) => {
	const mod = await importOriginal<typeof import('./client')>();
	return {
		...mod,
		apiGet: vi.fn(),
		apiPost: vi.fn()
	};
});

import { apiGet, apiPost } from './client';
import {
	createConversation,
	listConversations,
	listMessages,
	sendMessage,
	decide,
	revert
} from './agent';

const mockApiGet = vi.mocked(apiGet);
const mockApiPost = vi.mocked(apiPost);

const fakeConversation = {
	id: 1,
	title: 'Test',
	createdAt: '2026-01-01T00:00:00Z',
	updatedAt: '2026-01-01T00:00:00Z'
};

const fakeMessage = {
	id: 10,
	conversationId: 1,
	role: 'user' as const,
	content: [{ type: 'text', text: 'hello' }],
	createdAt: '2026-01-01T00:00:00Z'
};

beforeEach(() => {
	vi.resetAllMocks();
});

describe('createConversation', () => {
	it('calls apiPost /api/agent/conversations and returns the conversation', async () => {
		mockApiPost.mockResolvedValue(fakeConversation);
		const result = await createConversation();
		expect(mockApiPost).toHaveBeenCalledWith('/api/agent/conversations', {});
		expect(result).toEqual(fakeConversation);
	});

	it('calls apiPost with title when provided', async () => {
		mockApiPost.mockResolvedValue({ ...fakeConversation, title: 'My chat' });
		await createConversation('My chat');
		expect(mockApiPost).toHaveBeenCalledWith('/api/agent/conversations', { title: 'My chat' });
	});

	it('propagates ApiError with status 503', async () => {
		const err = new ApiError(503, 'AI not configured', { error: 'AI not configured' });
		mockApiPost.mockRejectedValue(err);
		await expect(createConversation()).rejects.toThrow(ApiError);
		await expect(createConversation()).rejects.toMatchObject({ status: 503 });
	});
});

describe('listConversations', () => {
	it('calls apiGet /api/agent/conversations and returns array', async () => {
		mockApiGet.mockResolvedValue([fakeConversation]);
		const result = await listConversations();
		expect(mockApiGet).toHaveBeenCalledWith('/api/agent/conversations');
		expect(result).toEqual([fakeConversation]);
	});

	it('coalesces null to empty array', async () => {
		mockApiGet.mockResolvedValue(null);
		const result = await listConversations();
		expect(result).toEqual([]);
	});
});

describe('listMessages', () => {
	it('calls apiGet /api/agent/conversations/7/messages', async () => {
		mockApiGet.mockResolvedValue([fakeMessage]);
		const result = await listMessages(7);
		expect(mockApiGet).toHaveBeenCalledWith('/api/agent/conversations/7/messages');
		expect(result).toEqual([fakeMessage]);
	});

	it('coalesces null to empty array', async () => {
		mockApiGet.mockResolvedValue(null);
		const result = await listMessages(7);
		expect(result).toEqual([]);
	});

	it('throws on invalid convId', async () => {
		await expect(listMessages(0)).rejects.toThrow();
		await expect(listMessages(-1)).rejects.toThrow();
	});
});

describe('sendMessage', () => {
	it('calls apiPost /api/agent/conversations/7/messages with {text}', async () => {
		mockApiPost.mockResolvedValue({ status: 'accepted', conversationId: 7 });
		await sendMessage(7, 'hi');
		expect(mockApiPost).toHaveBeenCalledWith('/api/agent/conversations/7/messages', { text: 'hi' });
	});

	it('returns void (ignores 202 body)', async () => {
		mockApiPost.mockResolvedValue({ status: 'accepted', conversationId: 7 });
		const result = await sendMessage(7, 'hello');
		expect(result).toBeUndefined();
	});

	it('throws on empty text', async () => {
		await expect(sendMessage(7, '')).rejects.toThrow();
	});

	it('propagates ApiError with status 503', async () => {
		const err = new ApiError(503, 'AI not configured', { error: 'AI not configured' });
		mockApiPost.mockRejectedValue(err);
		await expect(sendMessage(7, 'hi')).rejects.toThrow(ApiError);
		await expect(sendMessage(7, 'hi')).rejects.toMatchObject({ status: 503 });
	});
});

describe('decide', () => {
	it('calls apiPost /api/agent/steps/5/decision with {decision:"allow"}', async () => {
		mockApiPost.mockResolvedValue({ status: 'accepted', stepId: 5 });
		await decide(5, 'allow');
		expect(mockApiPost).toHaveBeenCalledWith('/api/agent/steps/5/decision', { decision: 'allow' });
	});

	it('calls apiPost with deny', async () => {
		mockApiPost.mockResolvedValue({ status: 'accepted', stepId: 5 });
		await decide(5, 'deny');
		expect(mockApiPost).toHaveBeenCalledWith('/api/agent/steps/5/decision', { decision: 'deny' });
	});

	it('returns void', async () => {
		mockApiPost.mockResolvedValue({ status: 'accepted', stepId: 5 });
		const result = await decide(5, 'allow');
		expect(result).toBeUndefined();
	});
});

describe('revert', () => {
	it('calls apiPost /api/agent/checkpoints/9/revert', async () => {
		const fakeResult = { conflicts: [{ table: 'invoices', pk: 42 }] };
		mockApiPost.mockResolvedValue(fakeResult);
		const result = await revert(9);
		expect(mockApiPost).toHaveBeenCalledWith('/api/agent/checkpoints/9/revert', {});
		expect(result).toEqual(fakeResult);
	});

	it('returns null when apiPost returns null', async () => {
		mockApiPost.mockResolvedValue(null);
		const result = await revert(9);
		expect(result).toBeNull();
	});
});
