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
	return chain;
}

const mockDb = createMockDb();

vi.mock('$lib/db/connection.js', () => ({
	getDb: vi.fn(() => mockDb)
}));

vi.mock('./parse.js', () => ({
	parseCsvFile: vi.fn(),
	validateRequiredField: vi.fn(),
	validateNumeric: vi.fn()
}));

import { parseCsvFile, validateRequiredField, validateNumeric } from './parse.js';
import { parseCatalogCsv, commitCatalogImport } from './import-catalog.js';

const mockParseCsvFile = vi.mocked(parseCsvFile);
const mockValidateRequiredField = vi.mocked(validateRequiredField);
const mockValidateNumeric = vi.mocked(validateNumeric);

beforeEach(() => {
	vi.clearAllMocks();
	mockValidateRequiredField.mockReturnValue(null);
	mockValidateNumeric.mockReturnValue(null);
	// Reset mock db chain
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

function makeFile(name = 'catalog.csv'): File {
	return new File([''], name, { type: 'text/csv' });
}

describe('parseCatalogCsv', () => {
	it('returns empty results for empty data', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		const result = await parseCatalogCsv(makeFile());
		expect(result.validRows).toEqual([]);
		expect(result.errors).toEqual([]);
		expect(result.totalRows).toBe(0);
		expect(result.skippedDuplicates).toBe(0);
	});

	it('returns valid rows when name is present and rate is valid', async () => {
		const rows = [{ uuid: 'u1', name: 'Consultation', rate: '150', unit: 'hr', category: 'Services', sku: 'CONS-001' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		const result = await parseCatalogCsv(makeFile());
		expect(result.validRows).toHaveLength(1);
		expect(result.totalRows).toBe(1);
	});

	it('collects name error when name is missing', async () => {
		const rows = [{ uuid: '', name: '', rate: '100', unit: '', category: '', sku: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockValidateRequiredField.mockReturnValueOnce({ row: 1, field: 'name', message: 'name is required' });
		const result = await parseCatalogCsv(makeFile());
		expect(result.errors).toHaveLength(1);
		expect(result.errors[0].field).toBe('name');
		expect(result.validRows).toHaveLength(0);
	});

	it('collects rate error when rate is invalid', async () => {
		const rows = [{ uuid: '', name: 'Item', rate: 'abc', unit: '', category: '', sku: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockValidateNumeric.mockReturnValueOnce({ row: 1, field: 'rate', message: 'rate must be a number' });
		const result = await parseCatalogCsv(makeFile());
		expect(result.errors).toHaveLength(1);
		expect(result.errors[0].field).toBe('rate');
	});

	it('skips duplicates when UUID already exists', async () => {
		const rows = [{ uuid: 'existing-uuid', name: 'Item A', rate: '100', unit: '', category: '', sku: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		// Mock the select query to return existing UUIDs
		mockDb.then = (resolve: any) => resolve([{ uuid: 'existing-uuid' }]);
		const result = await parseCatalogCsv(makeFile());
		expect(result.skippedDuplicates).toBe(1);
		expect(result.validRows).toHaveLength(0);
	});

	it('does not skip rows with new or empty UUIDs', async () => {
		const rows = [{ uuid: 'brand-new-uuid', name: 'Item B', rate: '50', unit: '', category: '', sku: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockDb.then = (resolve: any) => resolve([{ uuid: 'other-uuid' }]);
		const result = await parseCatalogCsv(makeFile());
		expect(result.skippedDuplicates).toBe(0);
		expect(result.validRows).toHaveLength(1);
	});

	it('rows with both name error and rate error are excluded', async () => {
		const rows = [{ uuid: '', name: '', rate: 'bad', unit: '', category: '', sku: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockValidateRequiredField.mockReturnValueOnce({ row: 1, field: 'name', message: 'name is required' });
		mockValidateNumeric.mockReturnValueOnce({ row: 1, field: 'rate', message: 'rate must be a number' });
		const result = await parseCatalogCsv(makeFile());
		expect(result.errors).toHaveLength(2);
		expect(result.validRows).toHaveLength(0);
	});
});

describe('commitCatalogImport', () => {
	it('calls db.insert for each row', async () => {
		const rows = [{ uuid: 'u1', name: 'Item A', rate: '100', unit: 'hr', category: 'Services', sku: 'S001' }];
		await commitCatalogImport(rows);
		expect(mockDb.insert).toHaveBeenCalled();
		expect(mockDb.values).toHaveBeenCalled();
	});

	it('inserts multiple rows', async () => {
		const rows = [
			{ uuid: 'u1', name: 'Item A', rate: '100', unit: 'hr', category: 'Services', sku: 'SKU-1' },
			{ uuid: 'u2', name: 'Item B', rate: '200', unit: 'ea', category: 'Products', sku: 'SKU-2' }
		];
		await commitCatalogImport(rows);
		expect(mockDb.insert).toHaveBeenCalledTimes(2);
	});

	it('handles empty rows without executing inserts', async () => {
		await commitCatalogImport([]);
		expect(mockDb.insert).not.toHaveBeenCalled();
	});

	it('defaults rate to 0 when empty or NaN', async () => {
		const rows = [{ uuid: 'u1', name: 'Item C', rate: '', unit: '', category: '', sku: '' }];
		await commitCatalogImport(rows);
		expect(mockDb.values).toHaveBeenCalledWith(
			expect.objectContaining({ rate: 0 })
		);
	});
});
