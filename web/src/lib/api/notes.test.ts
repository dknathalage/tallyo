import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the client module before importing notes.ts (same approach as agent.test.ts).
vi.mock('./client', async (importOriginal) => {
	const mod = await importOriginal<typeof import('./client')>();
	return {
		...mod,
		apiGet: vi.fn(),
		apiPost: vi.fn(),
		apiPut: vi.fn(),
		apiDelete: vi.fn()
	};
});

import { apiGet, apiPost, apiPut, apiDelete } from './client';
import { listForParticipant, create, update, remove, bill } from './notes';
import type { Note, NoteInput } from './types';

const mockApiGet = vi.mocked(apiGet);
const mockApiPost = vi.mocked(apiPost);
const mockApiPut = vi.mocked(apiPut);
const mockApiDelete = vi.mocked(apiDelete);

const fakeNote: Note = {
	id: 5,
	uuid: 'note-uuid',
	participantId: 3,
	serviceDate: '2026-06-10',
	body: 'visit note',
	transportKm: null,
	supportHours: null,
	authorUserId: null,
	billedInvoiceId: null,
	createdAt: '2026-06-10T00:00:00Z',
	updatedAt: '2026-06-10T00:00:00Z'
};

const fakeInput: NoteInput = {
	participantId: 3,
	serviceDate: '2026-06-10',
	body: 'visit note'
};

beforeEach(() => {
	vi.resetAllMocks();
});

describe('listForParticipant', () => {
	it('calls apiGet with the base path when no range params are given', async () => {
		mockApiGet.mockResolvedValue([fakeNote]);
		const result = await listForParticipant(3);
		expect(mockApiGet).toHaveBeenCalledWith('/api/participants/3/notes');
		expect(result).toEqual([fakeNote]);
	});

	it('appends from and to query params when provided', async () => {
		mockApiGet.mockResolvedValue([fakeNote]);
		await listForParticipant(3, '2026-06-09', '2026-06-12');
		expect(mockApiGet).toHaveBeenCalledWith(
			'/api/participants/3/notes?from=2026-06-09&to=2026-06-12'
		);
	});

	it('appends only from when to is omitted', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForParticipant(3, '2026-06-09');
		expect(mockApiGet).toHaveBeenCalledWith('/api/participants/3/notes?from=2026-06-09');
	});

	it('appends only to when from is omitted', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForParticipant(3, undefined, '2026-06-12');
		expect(mockApiGet).toHaveBeenCalledWith('/api/participants/3/notes?to=2026-06-12');
	});

	it('ignores empty-string range params (uses base path)', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForParticipant(3, '', '');
		expect(mockApiGet).toHaveBeenCalledWith('/api/participants/3/notes');
	});

	it('coalesces null to empty array', async () => {
		mockApiGet.mockResolvedValue(null);
		const result = await listForParticipant(3);
		expect(result).toEqual([]);
	});

	it('throws on a non-positive participantId', async () => {
		await expect(listForParticipant(0)).rejects.toThrow();
		await expect(listForParticipant(-1)).rejects.toThrow();
	});
});

describe('create', () => {
	it('calls apiPost /api/notes with the input and returns the note', async () => {
		mockApiPost.mockResolvedValue(fakeNote);
		const result = await create(fakeInput);
		expect(mockApiPost).toHaveBeenCalledWith('/api/notes', fakeInput);
		expect(result).toEqual(fakeNote);
	});

	it('throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(create(fakeInput)).rejects.toThrow();
	});

	it('throws on a non-positive participantId', async () => {
		await expect(create({ ...fakeInput, participantId: 0 })).rejects.toThrow();
	});
});

describe('update', () => {
	it('calls apiPut /api/notes/5 with the input and returns the note', async () => {
		mockApiPut.mockResolvedValue(fakeNote);
		const result = await update(5, fakeInput);
		expect(mockApiPut).toHaveBeenCalledWith('/api/notes/5', fakeInput);
		expect(result).toEqual(fakeNote);
	});

	it('throws when apiPut resolves null', async () => {
		mockApiPut.mockResolvedValue(null);
		await expect(update(5, fakeInput)).rejects.toThrow();
	});

	it('throws on a non-positive id', async () => {
		await expect(update(0, fakeInput)).rejects.toThrow();
	});
});

describe('remove', () => {
	it('calls apiDelete /api/notes/5', async () => {
		mockApiDelete.mockResolvedValue(null);
		await remove(5);
		expect(mockApiDelete).toHaveBeenCalledWith('/api/notes/5');
	});

	it('returns void', async () => {
		mockApiDelete.mockResolvedValue(null);
		const result = await remove(5);
		expect(result).toBeUndefined();
	});

	it('throws on a non-positive id', async () => {
		await expect(remove(0)).rejects.toThrow();
	});
});

describe('bill', () => {
	it('calls apiPost /api/notes/bill with {invoiceId, noteIds}', async () => {
		mockApiPost.mockResolvedValue(null);
		await bill(7, [1, 2, 3]);
		expect(mockApiPost).toHaveBeenCalledWith('/api/notes/bill', { invoiceId: 7, noteIds: [1, 2, 3] });
	});

	it('throws on a non-positive invoiceId', async () => {
		await expect(bill(0, [1])).rejects.toThrow();
	});

	it('throws on an empty noteIds array', async () => {
		await expect(bill(7, [])).rejects.toThrow();
	});
});
