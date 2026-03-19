import { describe, it, expect, vi, beforeEach } from 'vitest';

function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'onConflictDoUpdate'];
	for (const m of methods) {
		chain[m] = vi.fn().mockReturnValue(chain);
	}
	chain.then = (resolve: any) => resolve([]);
	chain[Symbol.iterator] = function* () {};
	chain.transaction = vi.fn(async (fn: any) => fn(chain));
	return chain;
}

const mockDb = createMockDb();

vi.mock('../connection.js', () => ({
	getDb: vi.fn(() => mockDb)
}));

vi.mock('../audit.js', () => ({
	logAudit: vi.fn().mockResolvedValue(undefined),
	computeChanges: vi.fn().mockReturnValue({})
}));

import { getBusinessProfile, saveBusinessProfile, buildBusinessSnapshot } from './business-profile.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy', 'onConflictDoUpdate']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getBusinessProfile', () => {
	it('is an async function', () => {
		expect(getBusinessProfile()).toBeInstanceOf(Promise);
	});

	it('returns null when no profile exists', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getBusinessProfile();
		expect(result).toBeNull();
	});

	it('returns profile when exists', async () => {
		const profile = { id: 1, name: 'My Business', email: 'biz@test.com' };
		mockDb.then = (resolve: any) => resolve([profile]);
		const result = await getBusinessProfile();
		expect(result).toBeDefined();
	});
});

describe('saveBusinessProfile', () => {
	it('is an async function', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		expect(saveBusinessProfile({ name: 'My Biz', email: 'biz@test.com' })).toBeInstanceOf(Promise);
	});
});

describe('buildBusinessSnapshot', () => {
	it('is an async function', () => {
		mockDb.then = (resolve: any) => resolve([]);
		expect(buildBusinessSnapshot()).toBeInstanceOf(Promise);
	});

	it('returns empty snapshot when no profile', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const snapshot = await buildBusinessSnapshot();
		expect(snapshot).toEqual({
			name: '',
			email: '',
			phone: '',
			address: '',
			metadata: {}
		});
	});
});
