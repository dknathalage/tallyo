import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the client module before importing shifts.ts (same approach as notes.test.ts).
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
import {
	listAll,
	listForParticipant,
	suggestions,
	toRecord,
	create,
	update,
	remove,
	setStatus,
	importShifts,
	draftInvoice
} from './shifts';
import type { Shift, ShiftInput } from './types';

const mockApiGet = vi.mocked(apiGet);
const mockApiPost = vi.mocked(apiPost);
const mockApiPut = vi.mocked(apiPut);
const mockApiDelete = vi.mocked(apiDelete);

const fakeShift: Shift = {
	id: 5,
	uuid: 'shift-uuid',
	participantId: 3,
	serviceDate: '2026-06-10',
	startTime: '09:00',
	endTime: '15:00',
	hours: 6,
	km: 20,
	measures: [],
	note: 'cleaning, laundry',
	tags: [],
	status: 'recorded',
	invoiceId: null,
	authorUserId: null,
	createdAt: '2026-06-10T00:00:00Z',
	updatedAt: '2026-06-10T00:00:00Z'
};

const fakeInput: ShiftInput = {
	participantId: 3,
	serviceDate: '2026-06-10',
	startTime: '09:00',
	endTime: '15:00',
	hours: 6,
	km: 20,
	measures: [],
	note: 'cleaning, laundry',
	tags: [],
	status: 'recorded'
};

beforeEach(() => {
	vi.resetAllMocks();
});

describe('listAll', () => {
	it('calls apiGet /api/shifts and returns the list', async () => {
		mockApiGet.mockResolvedValue([fakeShift]);
		const result = await listAll();
		expect(mockApiGet).toHaveBeenCalledWith('/api/shifts');
		expect(result).toEqual([fakeShift]);
	});

	it('coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await listAll()).toEqual([]);
	});
});

describe('listForParticipant', () => {
	it('uses the base path with no params', async () => {
		mockApiGet.mockResolvedValue([fakeShift]);
		await listForParticipant(3);
		expect(mockApiGet).toHaveBeenCalledWith('/api/participants/3/shifts');
	});

	it('appends from, to and status when provided', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForParticipant(3, '2026-06-01', '2026-06-30', 'recorded');
		expect(mockApiGet).toHaveBeenCalledWith(
			'/api/participants/3/shifts?from=2026-06-01&to=2026-06-30&status=recorded'
		);
	});

	it('throws on a non-positive participantId', async () => {
		await expect(listForParticipant(0)).rejects.toThrow();
	});
});

describe('suggestions / toRecord', () => {
	it('suggestions calls /api/shifts/suggestions and coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await suggestions()).toEqual([]);
		expect(mockApiGet).toHaveBeenCalledWith('/api/shifts/suggestions');
	});

	it('toRecord calls /api/shifts/to-record', async () => {
		mockApiGet.mockResolvedValue([fakeShift]);
		expect(await toRecord()).toEqual([fakeShift]);
		expect(mockApiGet).toHaveBeenCalledWith('/api/shifts/to-record');
	});
});

describe('create / update / remove', () => {
	it('create posts to /api/shifts and returns the shift', async () => {
		mockApiPost.mockResolvedValue(fakeShift);
		const result = await create(fakeInput);
		expect(mockApiPost).toHaveBeenCalledWith('/api/shifts', fakeInput);
		expect(result).toEqual(fakeShift);
	});

	it('create throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(create(fakeInput)).rejects.toThrow();
	});

	it('update puts to /api/shifts/5', async () => {
		mockApiPut.mockResolvedValue(fakeShift);
		await update(5, fakeInput);
		expect(mockApiPut).toHaveBeenCalledWith('/api/shifts/5', fakeInput);
	});

	it('remove deletes /api/shifts/5', async () => {
		mockApiDelete.mockResolvedValue(null);
		await remove(5);
		expect(mockApiDelete).toHaveBeenCalledWith('/api/shifts/5');
	});

	it('create throws on a missing service date', async () => {
		await expect(create({ ...fakeInput, serviceDate: '' })).rejects.toThrow();
	});
});

describe('setStatus', () => {
	it('posts {status} to /api/shifts/5/status', async () => {
		mockApiPost.mockResolvedValue(null);
		await setStatus(5, 'sent');
		expect(mockApiPost).toHaveBeenCalledWith('/api/shifts/5/status', { status: 'sent' });
	});

	it('throws on a non-positive id', async () => {
		await expect(setStatus(0, 'sent')).rejects.toThrow();
	});
});

describe('importShifts', () => {
	it('posts {participantId, text} to /api/shifts/import and returns created shifts', async () => {
		mockApiPost.mockResolvedValue([fakeShift]);
		const result = await importShifts(3, 'Mon 9-3 cleaning');
		expect(mockApiPost).toHaveBeenCalledWith('/api/shifts/import', {
			participantId: 3,
			text: 'Mon 9-3 cleaning'
		});
		expect(result).toEqual([fakeShift]);
	});

	it('throws on blank text', async () => {
		await expect(importShifts(3, '   ')).rejects.toThrow();
	});
});

describe('draftInvoice', () => {
	it('posts {from, to} to /api/participants/3/draft-invoice', async () => {
		const inv = { id: 9 } as unknown;
		mockApiPost.mockResolvedValue(inv);
		await draftInvoice(3, '2026-06-01', '2026-06-30');
		expect(mockApiPost).toHaveBeenCalledWith('/api/participants/3/draft-invoice', {
			from: '2026-06-01',
			to: '2026-06-30'
		});
	});

	it('throws when from/to missing', async () => {
		await expect(draftInvoice(3, '', '2026-06-30')).rejects.toThrow();
	});

	it('throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(draftInvoice(3, '2026-06-01', '2026-06-30')).rejects.toThrow();
	});
});
