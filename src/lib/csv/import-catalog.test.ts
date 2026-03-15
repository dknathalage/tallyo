import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	runRaw: vi.fn(),
	save: vi.fn()
}));

vi.mock('./parse.js', () => ({
	parseCsvFile: vi.fn(),
	validateRequiredField: vi.fn(),
	validateNumeric: vi.fn()
}));

import { query, execute, runRaw } from '$lib/db/connection.js';
import { parseCsvFile, validateRequiredField, validateNumeric } from './parse.js';
import { parseCatalogCsv, commitCatalogImport } from './import-catalog.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockRunRaw = vi.mocked(runRaw);
const mockParseCsvFile = vi.mocked(parseCsvFile);
const mockValidateRequiredField = vi.mocked(validateRequiredField);
const mockValidateNumeric = vi.mocked(validateNumeric);

beforeEach(() => {
	vi.clearAllMocks();
	mockValidateRequiredField.mockReturnValue(null);
	mockValidateNumeric.mockReturnValue(null);
	mockQuery.mockReturnValue([]);
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
		mockQuery.mockReturnValue([{ uuid: 'existing-uuid' }]);
		const result = await parseCatalogCsv(makeFile());
		expect(result.skippedDuplicates).toBe(1);
		expect(result.validRows).toHaveLength(0);
	});

	it('does not skip rows with new or empty UUIDs', async () => {
		const rows = [{ uuid: 'brand-new-uuid', name: 'Item B', rate: '50', unit: '', category: '', sku: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockQuery.mockReturnValue([{ uuid: 'other-uuid' }]);
		const result = await parseCatalogCsv(makeFile());
		expect(result.skippedDuplicates).toBe(0);
		expect(result.validRows).toHaveLength(1);
	});

	it('queries existing UUIDs from catalog_items', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		await parseCatalogCsv(makeFile());
		expect(mockQuery).toHaveBeenCalledWith(expect.stringContaining('SELECT uuid FROM catalog_items'));
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
	it('wraps inserts in a transaction', async () => {
		const rows = [{ uuid: 'u1', name: 'Item A', rate: '100', unit: 'hr', category: 'Services', sku: 'S001' }];
		await commitCatalogImport(rows);
		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
	});

	it('inserts each row into catalog_items', async () => {
		const rows = [{ uuid: 'u1', name: 'Item A', rate: '99.99', unit: 'hr', category: 'Cat A', sku: 'SKU-1' }];
		await commitCatalogImport(rows);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO catalog_items'),
			expect.arrayContaining(['u1', 'Item A', 99.99, 'hr', 'Cat A', 'SKU-1'])
		);
	});

	it('generates a UUID when uuid field is empty', async () => {
		const rows = [{ uuid: '', name: 'Item B', rate: '50', unit: '', category: '', sku: '' }];
		await commitCatalogImport(rows);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO catalog_items'),
			expect.arrayContaining(['Item B'])
		);
		const callArgs = mockExecute.mock.calls[0][1] as unknown[];
		expect(typeof callArgs[0]).toBe('string');
		expect((callArgs[0] as string).length).toBeGreaterThan(0);
	});

	it('defaults rate to 0 when empty or NaN', async () => {
		const rows = [{ uuid: 'u1', name: 'Item C', rate: '', unit: '', category: '', sku: '' }];
		await commitCatalogImport(rows);
		expect(mockExecute).toHaveBeenCalledWith(expect.any(String), expect.arrayContaining([0]));
	});

	it('handles empty rows without executing inserts', async () => {
		await commitCatalogImport([]);
		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).not.toHaveBeenCalled();
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
	});

	it('rolls back on error and rethrows', async () => {
		const rows = [{ uuid: 'u1', name: 'Item A', rate: '100', unit: '', category: '', sku: '' }];
		mockExecute.mockImplementationOnce(() => { throw new Error('DB error'); });
		await expect(commitCatalogImport(rows)).rejects.toThrow('DB error');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});

	it('inserts multiple rows', async () => {
		const rows = [
			{ uuid: 'u1', name: 'Item A', rate: '100', unit: 'hr', category: 'Services', sku: 'SKU-1' },
			{ uuid: 'u2', name: 'Item B', rate: '200', unit: 'ea', category: 'Products', sku: 'SKU-2' }
		];
		await commitCatalogImport(rows);
		expect(mockExecute).toHaveBeenCalledTimes(2);
	});

	it('uses trimmed field values', async () => {
		const rows = [{ uuid: ' u1 ', name: '  Item A  ', rate: '100', unit: '  hr  ', category: '  Cat  ', sku: '  SKU  ' }];
		await commitCatalogImport(rows);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.any(String),
			expect.arrayContaining(['u1', 'Item A', 100, 'hr', 'Cat', 'SKU'])
		);
	});
});
