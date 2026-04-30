import { describe, it, expect, vi, beforeEach } from 'vitest';

function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy'];
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

import {
	getColumnMappings,
	getColumnMapping,
	createColumnMapping,
	updateColumnMapping,
	deleteColumnMapping
} from './column-mappings.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getColumnMappings', () => {
	it('is an async function', () => {
		expect(getColumnMappings()).toBeInstanceOf(Promise);
	});

	it('returns empty array when no mappings exist', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getColumnMappings();
		expect(result).toEqual([]);
	});
});

describe('getColumnMapping', () => {
	it('is an async function', () => {
		expect(getColumnMapping(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getColumnMapping(999);
		expect(result).toBeNull();
	});
});

describe('createColumnMapping', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 5 }]);
		expect(createColumnMapping({
			name: 'My Mapping',
			entity_type: 'invoice',
			mapping: { col1: 'field1' }
		})).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 5 }]);
		const id = await createColumnMapping({
			name: 'My Mapping',
			mapping: { a: 'b' }
		});
		expect(id).toBe(5);
	});
});

describe('updateColumnMapping', () => {
	it('is an async function', () => {
		expect(updateColumnMapping(3, {
			name: 'Updated',
			entity_type: 'client',
			mapping: { x: 'y' }
		})).toBeInstanceOf(Promise);
	});
});

describe('deleteColumnMapping', () => {
	it('is an async function', () => {
		expect(deleteColumnMapping(7)).toBeInstanceOf(Promise);
	});
});
