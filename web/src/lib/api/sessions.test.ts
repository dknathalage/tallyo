import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the client module before importing sessions.ts.
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
	listForClient,
	suggestions,
	toRecord,
	create,
	update,
	remove,
	setStatus,
	draftFromSessions,
	listItems,
	addItem,
	updateItem,
	deleteItem
} from './sessions';
import type { Session, SessionInput, LineItem, LineItemInput } from './types';

const mockApiGet = vi.mocked(apiGet);
const mockApiPost = vi.mocked(apiPost);
const mockApiPut = vi.mocked(apiPut);
const mockApiPatch = vi.mocked(apiPatch);
const mockApiDelete = vi.mocked(apiDelete);

const SESSION_UUID = '11111111-1111-4111-8111-111111111111';
const PART_UUID = '22222222-2222-4222-8222-222222222222';
const ITEM_UUID = '33333333-3333-4333-8333-333333333333';

const fakeSession: Session = {
	id: SESSION_UUID,
	clientId: PART_UUID,
	serviceDate: '2026-06-10',
	note: 'cleaning, laundry',
	tags: [],
	status: 'recorded',
	invoiceId: null,
	createdAt: '2026-06-10T00:00:00Z',
	updatedAt: '2026-06-10T00:00:00Z'
};

const fakeInput: SessionInput = {
	clientId: PART_UUID,
	serviceDate: '2026-06-10',
	note: 'cleaning, laundry',
	tags: [],
	status: 'recorded'
};

const fakeItemInput: LineItemInput = {
	itemId: null,
	customItemId: null,
	priceListVersionId: null,
	code: '01_011_0107_1_1',
	description: 'self-care',
	serviceDate: '2026-06-10',
	unit: 'H',
	startTime: '',
	endTime: '',
	quantity: 7,
	unitPrice: 0,
	taxable: false,
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
	it('calls apiGet /api/sessions and returns the list', async () => {
		mockApiGet.mockResolvedValue([fakeSession]);
		const result = await listAll();
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/sessions');
		expect(result).toEqual([fakeSession]);
	});

	it('coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await listAll()).toEqual([]);
	});
});

describe('listForClient', () => {
	it('filters by the client uuid', async () => {
		mockApiGet.mockResolvedValue([fakeSession]);
		await listForClient(PART_UUID);
		expect(mockApiGet).toHaveBeenCalledWith(`/api/t/t-uuid/sessions?client=${PART_UUID}`);
	});

	it('appends from, to and status when provided', async () => {
		mockApiGet.mockResolvedValue([]);
		await listForClient(PART_UUID, '2026-06-01', '2026-06-30', 'recorded');
		expect(mockApiGet).toHaveBeenCalledWith(
			`/api/t/t-uuid/sessions?client=${PART_UUID}&from=2026-06-01&to=2026-06-30&status=recorded`
		);
	});

	it('throws on an empty clientId', async () => {
		await expect(listForClient('')).rejects.toThrow();
	});
});

describe('suggestions / toRecord', () => {
	it('suggestions calls /api/sessions/suggestions and coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await suggestions()).toEqual([]);
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/sessions/suggestions');
	});

	it('toRecord calls /api/sessions/to-record', async () => {
		mockApiGet.mockResolvedValue([fakeSession]);
		expect(await toRecord()).toEqual([fakeSession]);
		expect(mockApiGet).toHaveBeenCalledWith('/api/t/t-uuid/sessions/to-record');
	});
});

describe('create / update / remove', () => {
	it('create posts to /api/sessions and returns the session', async () => {
		mockApiPost.mockResolvedValue(fakeSession);
		const result = await create(fakeInput);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/sessions', fakeInput);
		expect(result).toEqual(fakeSession);
	});

	it('create throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(create(fakeInput)).rejects.toThrow();
	});

	it('update puts to /api/sessions/{uuid}', async () => {
		mockApiPut.mockResolvedValue(fakeSession);
		await update(SESSION_UUID, fakeInput);
		expect(mockApiPut).toHaveBeenCalledWith(`/api/t/t-uuid/sessions/${SESSION_UUID}`, fakeInput);
	});

	it('remove deletes /api/sessions/{uuid}', async () => {
		mockApiDelete.mockResolvedValue(null);
		await remove(SESSION_UUID);
		expect(mockApiDelete).toHaveBeenCalledWith(`/api/t/t-uuid/sessions/${SESSION_UUID}`);
	});

	it('create throws on a missing service date', async () => {
		await expect(create({ ...fakeInput, serviceDate: '' })).rejects.toThrow();
	});
});

describe('setStatus', () => {
	it('posts {status} to /api/sessions/{uuid}/status', async () => {
		mockApiPost.mockResolvedValue(null);
		await setStatus(SESSION_UUID, 'sent');
		expect(mockApiPost).toHaveBeenCalledWith(`/api/t/t-uuid/sessions/${SESSION_UUID}/status`, {
			status: 'sent'
		});
	});

	it('throws on an empty id', async () => {
		await expect(setStatus('', 'sent')).rejects.toThrow();
	});
});

describe('draftFromSessions', () => {
	it('posts {sessionIds} to /api/invoices/draft-from-sessions', async () => {
		mockApiPost.mockResolvedValue({ id: 'inv-uuid' } as unknown);
		await draftFromSessions([SESSION_UUID, PART_UUID]);
		expect(mockApiPost).toHaveBeenCalledWith('/api/t/t-uuid/invoices/draft-from-sessions', {
			sessionIds: [SESSION_UUID, PART_UUID]
		});
	});

	it('throws on an empty sessionIds list', async () => {
		await expect(draftFromSessions([])).rejects.toThrow();
	});

	it('throws when apiPost resolves null', async () => {
		mockApiPost.mockResolvedValue(null);
		await expect(draftFromSessions([SESSION_UUID])).rejects.toThrow();
	});
});

describe('session items', () => {
	it('listItems gets /api/sessions/{uuid}/items and coalesces null to []', async () => {
		mockApiGet.mockResolvedValue(null);
		expect(await listItems(SESSION_UUID)).toEqual([]);
		expect(mockApiGet).toHaveBeenCalledWith(`/api/t/t-uuid/sessions/${SESSION_UUID}/items`);
	});

	it('addItem posts to /api/sessions/{uuid}/items', async () => {
		mockApiPost.mockResolvedValue(fakeItem);
		const result = await addItem(SESSION_UUID, fakeItemInput);
		expect(mockApiPost).toHaveBeenCalledWith(`/api/t/t-uuid/sessions/${SESSION_UUID}/items`, fakeItemInput);
		expect(result).toEqual(fakeItem);
	});

	it('updateItem patches /api/sessions/{uuid}/items/{itemUuid}', async () => {
		mockApiPatch.mockResolvedValue(fakeItem);
		await updateItem(SESSION_UUID, ITEM_UUID, fakeItemInput);
		expect(mockApiPatch).toHaveBeenCalledWith(
			`/api/t/t-uuid/sessions/${SESSION_UUID}/items/${ITEM_UUID}`,
			fakeItemInput
		);
	});

	it('deleteItem deletes /api/sessions/{uuid}/items/{itemUuid}', async () => {
		mockApiDelete.mockResolvedValue(null);
		await deleteItem(SESSION_UUID, ITEM_UUID);
		expect(mockApiDelete).toHaveBeenCalledWith(
			`/api/t/t-uuid/sessions/${SESSION_UUID}/items/${ITEM_UUID}`
		);
	});
});
