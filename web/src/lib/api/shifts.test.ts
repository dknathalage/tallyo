import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the client module before importing shifts.ts.
vi.mock('./client', async (importOriginal) => {
	const mod = await importOriginal<typeof import('./client')>();
	return {
		...mod,
		apiGet: vi.fn(),
		apiPost: vi.fn(),
		apiPut: vi.fn(),
		apiPatch: vi.fn(),
		apiDelete: vi.fn()
	};
});

import { apiGet, apiPost, apiPut, apiPatch, apiDelete, setActiveTenant } from './client';
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
	draftFromShifts,
	listItems,
	addItem,
	updateItem,
	deleteItem,
	divideShift
} from './shifts';
import type { Shift, ShiftInput, LineItem, LineItemInput } from './types';

const mockApiGet = vi.mocked(apiGet);
const mockApiPost = vi.mocked(apiPost);
const mockApiPut = vi.mocked(apiPut);
const mockApiPatch = vi.mocked(apiPatch);
const mockApiDelete = vi.mocked(apiDelete);

const fakeShift: Shift = {
	id: 5,
	uuid: 'shift-uuid',
	participantId: 3,
	serviceDate: '2026-06-10',
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
	note: 'cleaning, laundry',
	tags: [],
	status: 'recorded'
};

const fakeItemInput: LineItemInput = {
	supportItemId: null,
	customItemId: null,
	catalogVersionId: null,
	code: '01_011_0107_1_1',
	description: 'self-care',
	serviceDate: '2026-06-10',
	unit: 'H',
	startTime: '',
	endTime: '',
	quantity: 7,
	unitPrice: 0,
	gstFree: true,
	sortOrder: 0
};

const fakeItem = { id: 11, ...fakeItemInput, lineTotal: 0 } as unknown as LineItem;

beforeEach(() => {
	vi.resetAllMocks();
	// Real (unmocked) setActiveTenant/tenantPath: every tenant-scoped call now
	// resolves to /api/t/t-uuid/... — assert against the prefixed URL.
	setActiveTenant('t-uuid');
});

describe('listAll', () => {
	it('calls apiGet /api/shifts and returns the list', async () => {
		mockApiGet.mockResolvedValue([fakeShift]);
		const result = await listAll();
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/shifts');
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
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/participants/3/shifts');
	});

	it('appends from, to and status when provided', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForParticipant(3, '2026-06-01', '2026-06-30', 'recorded');
		expect(mockApiGet).toHaveBeenCalledWith(
			'/api/t/t-uuid/participants/3/shifts?from=2026-06-01&to=2026-06-30&status=recorded'
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
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/shifts/suggestions');
	});

	it('toRecord calls /api/shifts/to-record', async () => {
		mockApiGet.mockResolvedValue([fakeShift]);
		expect(await toRecord()).toEqual([fakeShift]);
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/shifts/to-record');
	});
});

describe('create / update / remove', () => {
	it('create posts to /api/shifts and returns the shift', async () => {
		mockApiPost.mockResolvedValue(fakeShift);
		const result = await create(fakeInput);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/shifts', fakeInput);
		expect(result).toEqual(fakeShift);
	});

	it('create throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(create(fakeInput)).rejects.toThrow();
	});

	it('update puts to /api/shifts/5', async () => {
		mockApiPut.mockResolvedValue(fakeShift);
		await update(5, fakeInput);
		expect(mockApiPut).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5', fakeInput);
	});

	it('remove deletes /api/shifts/5', async () => {
		mockApiDelete.mockResolvedValue(null);
		await remove(5);
		expect(mockApiDelete).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5');
	});

	it('create throws on a missing service date', async () => {
		await expect(create({ ...fakeInput, serviceDate: '' })).rejects.toThrow();
	});
});

describe('setStatus', () => {
	it('posts {status} to /api/shifts/5/status', async () => {
		mockApiPost.mockResolvedValue(null);
		await setStatus(5, 'sent');
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5/status', { status: 'sent' });
	});

	it('throws on a non-positive id', async () => {
		await expect(setStatus(0, 'sent')).rejects.toThrow();
	});
});

describe('importShifts', () => {
	it('posts {participantId, text} to /api/shifts/import and returns created shifts', async () => {
		mockApiPost.mockResolvedValue([fakeShift]);
		const result = await importShifts(3, 'Mon 9-3 cleaning');
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/shifts/import', {
			participantId: 3,
			text: 'Mon 9-3 cleaning'
		});
		expect(result).toEqual([fakeShift]);
	});

	it('throws on blank text', async () => {
		await expect(importShifts(3, '   ')).rejects.toThrow();
	});
});

describe('draftFromShifts', () => {
	it('posts {shiftIds} to /api/invoices/draft-from-shifts', async () => {
		mockApiPost.mockResolvedValue({ id: 9 } as unknown);
		await draftFromShifts([5, 6]);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/invoices/draft-from-shifts', {
			shiftIds: [5, 6]
		});
	});

	it('throws on an empty shiftIds list', async () => {
		await expect(draftFromShifts([])).rejects.toThrow();
	});

	it('throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(draftFromShifts([5])).rejects.toThrow();
	});
});

describe('shift items', () => {
	it('listItems gets /api/shifts/5/items and coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await listItems(5)).toEqual([]);
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5/items');
	});

	it('addItem posts to /api/shifts/5/items', async () => {
		mockApiPost.mockResolvedValue(fakeItem);
		const result = await addItem(5, fakeItemInput);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5/items', fakeItemInput);
		expect(result).toEqual(fakeItem);
	});

	it('updateItem patches /api/shifts/5/items/11', async () => {
		mockApiPatch.mockResolvedValue(fakeItem);
		await updateItem(5, 11, fakeItemInput);
		expect(mockApiPatch).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5/items/11', fakeItemInput);
	});

	it('deleteItem deletes /api/shifts/5/items/11', async () => {
		mockApiDelete.mockResolvedValue(null);
		await deleteItem(5, 11);
		expect(mockApiDelete).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5/items/11');
	});

	it('divideShift posts to /api/shifts/5/divide and coalesces null to []', async () => {
		mockApiPost.mockResolvedValue(null);
		expect(await divideShift(5)).toEqual([]);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/shifts/5/divide', {});
	});
});
