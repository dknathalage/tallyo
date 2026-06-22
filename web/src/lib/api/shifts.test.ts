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

const SHIFT_UUID = '11111111-1111-4111-8111-111111111111';
const PART_UUID = '22222222-2222-4222-8222-222222222222';
const ITEM_UUID = '33333333-3333-4333-8333-333333333333';

const fakeShift: Shift = {
	id: SHIFT_UUID,
	participantId: PART_UUID,
	serviceDate: '2026-06-10',
	note: 'cleaning, laundry',
	tags: [],
	status: 'recorded',
	invoiceId: null,
	createdAt: '2026-06-10T00:00:00Z',
	updatedAt: '2026-06-10T00:00:00Z'
};

const fakeInput: ShiftInput = {
	participantId: PART_UUID,
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

const fakeItem = { id: ITEM_UUID, ...fakeItemInput, lineTotal: 0 } as unknown as LineItem;

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
	it('filters by the participant uuid', async () => {
		mockApiGet.mockResolvedValue([fakeShift]);
		await listForParticipant(PART_UUID);
		expect(mockApiGet).toHaveBeenCalledWith(`/api/t/t-uuid/shifts?participant=${PART_UUID}`);
	});

	it('appends from, to and status when provided', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForParticipant(PART_UUID, '2026-06-01', '2026-06-30', 'recorded');
		expect(mockApiGet).toHaveBeenCalledWith(
			`/api/t/t-uuid/shifts?participant=${PART_UUID}&from=2026-06-01&to=2026-06-30&status=recorded`
		);
	});

	it('throws on an empty participantId', async () => {
		await expect(listForParticipant('')).rejects.toThrow();
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

	it('update puts to /api/shifts/{uuid}', async () => {
		mockApiPut.mockResolvedValue(fakeShift);
		await update(SHIFT_UUID, fakeInput);
		expect(mockApiPut).toHaveBeenCalledWith(`/api/t/t-uuid/shifts/${SHIFT_UUID}`, fakeInput);
	});

	it('remove deletes /api/shifts/{uuid}', async () => {
		mockApiDelete.mockResolvedValue(null);
		await remove(SHIFT_UUID);
		expect(mockApiDelete).toHaveBeenCalledWith(`/api/t/t-uuid/shifts/${SHIFT_UUID}`);
	});

	it('create throws on a missing service date', async () => {
		await expect(create({ ...fakeInput, serviceDate: '' })).rejects.toThrow();
	});
});

describe('setStatus', () => {
	it('posts {status} to /api/shifts/{uuid}/status', async () => {
		mockApiPost.mockResolvedValue(null);
		await setStatus(SHIFT_UUID, 'sent');
		expect(mockApiPost).toHaveBeenCalledWith(`/api/t/t-uuid/shifts/${SHIFT_UUID}/status`, {
			status: 'sent'
		});
	});

	it('throws on an empty id', async () => {
		await expect(setStatus('', 'sent')).rejects.toThrow();
	});
});

describe('importShifts', () => {
	it('posts {participantId, text} to /api/shifts/import and returns created shifts', async () => {
		mockApiPost.mockResolvedValue([fakeShift]);
		const result = await importShifts(PART_UUID, 'Mon 9-3 cleaning');
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/shifts/import', {
			participantId: PART_UUID,
			text: 'Mon 9-3 cleaning'
		});
		expect(result).toEqual([fakeShift]);
	});

	it('throws on blank text', async () => {
		await expect(importShifts(PART_UUID, '   ')).rejects.toThrow();
	});
});

describe('draftFromShifts', () => {
	it('posts {shiftIds} to /api/invoices/draft-from-shifts', async () => {
		mockApiPost.mockResolvedValue({ id: 'inv-uuid' } as unknown);
		await draftFromShifts([SHIFT_UUID, PART_UUID]);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/invoices/draft-from-shifts', {
			shiftIds: [SHIFT_UUID, PART_UUID]
		});
	});

	it('throws on an empty shiftIds list', async () => {
		await expect(draftFromShifts([])).rejects.toThrow();
	});

	it('throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(draftFromShifts([SHIFT_UUID])).rejects.toThrow();
	});
});

describe('shift items', () => {
	it('listItems gets /api/shifts/{uuid}/items and coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await listItems(SHIFT_UUID)).toEqual([]);
		expect(mockApiGet).toHaveBeenCalledWith(`/api/t/t-uuid/shifts/${SHIFT_UUID}/items`);
	});

	it('addItem posts to /api/shifts/{uuid}/items', async () => {
		mockApiPost.mockResolvedValue(fakeItem);
		const result = await addItem(SHIFT_UUID, fakeItemInput);
		expect(mockApiPost).toHaveBeenCalledWith(`/api/t/t-uuid/shifts/${SHIFT_UUID}/items`, fakeItemInput);
		expect(result).toEqual(fakeItem);
	});

	it('updateItem patches /api/shifts/{uuid}/items/{itemUuid}', async () => {
		mockApiPatch.mockResolvedValue(fakeItem);
		await updateItem(SHIFT_UUID, ITEM_UUID, fakeItemInput);
		expect(mockApiPatch).toHaveBeenCalledWith(
			`/api/t/t-uuid/shifts/${SHIFT_UUID}/items/${ITEM_UUID}`,
			fakeItemInput
		);
	});

	it('deleteItem deletes /api/shifts/{uuid}/items/{itemUuid}', async () => {
		mockApiDelete.mockResolvedValue(null);
		await deleteItem(SHIFT_UUID, ITEM_UUID);
		expect(mockApiDelete).toHaveBeenCalledWith(
			`/api/t/t-uuid/shifts/${SHIFT_UUID}/items/${ITEM_UUID}`
		);
	});

	it('divideShift posts to /api/shifts/{uuid}/divide and coalesces null to []', async () => {
		mockApiPost.mockResolvedValue(null);
		expect(await divideShift(SHIFT_UUID)).toEqual([]);
		expect(mockApiPost).toHaveBeenCalledWith(`/api/t/t-uuid/shifts/${SHIFT_UUID}/divide`, {});
	});
});
