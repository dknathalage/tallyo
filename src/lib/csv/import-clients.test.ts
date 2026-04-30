import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./parse.js', () => ({
	parseCsvFile: vi.fn(),
	validateRequiredField: vi.fn()
}));

import { parseCsvFile, validateRequiredField } from './parse.js';
import { parseClientsCsv, commitClientImport } from './import-clients.js';

const mockParseCsvFile = vi.mocked(parseCsvFile);
const mockValidateRequiredField = vi.mocked(validateRequiredField);
const mockFetch = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
	mockValidateRequiredField.mockReturnValue(null);
});

function makeFile(name = 'clients.csv'): File {
	return new File([''], name, { type: 'text/csv' });
}

describe('parseClientsCsv', () => {
	it('returns empty valid rows and no errors for empty data', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseClientsCsv(makeFile());
		expect(result.validRows).toEqual([]);
		expect(result.errors).toEqual([]);
		expect(result.totalRows).toBe(0);
		expect(result.skippedDuplicates).toBe(0);
	});

	it('returns valid rows when name is present', async () => {
		const rows = [{ uuid: 'uuid-1', name: 'Alice', email: 'alice@example.com', phone: '', address: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseClientsCsv(makeFile());
		expect(result.validRows).toHaveLength(1);
		expect(result.validRows[0]!.name).toBe('Alice');
	});

	it('collects errors when name is missing', async () => {
		const rows = [{ uuid: '', name: '', email: '', phone: '', address: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		mockValidateRequiredField.mockReturnValue({ row: 1, field: 'name', message: 'name is required' });
		const result = await parseClientsCsv(makeFile());
		expect(result.errors).toHaveLength(1);
		expect(result.errors[0]!.field).toBe('name');
		expect(result.validRows).toHaveLength(0);
	});

	it('skips duplicates when UUID already exists', async () => {
		const rows = [{ uuid: 'existing-uuid', name: 'Alice', email: '', phone: '', address: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ uuid: 'existing-uuid' }]) });
		const result = await parseClientsCsv(makeFile());
		expect(result.skippedDuplicates).toBe(1);
		expect(result.validRows).toHaveLength(0);
	});

	it('does not skip when UUID is new', async () => {
		const rows = [{ uuid: 'new-uuid', name: 'Bob', email: '', phone: '', address: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ uuid: 'different-uuid' }]) });
		const result = await parseClientsCsv(makeFile());
		expect(result.skippedDuplicates).toBe(0);
		expect(result.validRows).toHaveLength(1);
	});

	it('does not skip when UUID is empty (no dedup key)', async () => {
		const rows = [{ uuid: '', name: 'Charlie', email: '', phone: '', address: '' }];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseClientsCsv(makeFile());
		expect(result.skippedDuplicates).toBe(0);
		expect(result.validRows).toHaveLength(1);
	});

	it('returns correct totalRows count', async () => {
		const rows = [
			{ uuid: '1', name: 'Alice', email: '', phone: '', address: '' },
			{ uuid: '2', name: 'Bob', email: '', phone: '', address: '' },
			{ uuid: '3', name: 'Charlie', email: '', phone: '', address: '' }
		];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseClientsCsv(makeFile());
		expect(result.totalRows).toBe(3);
	});

	it('fetches from /api/clients for deduplication', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await parseClientsCsv(makeFile());
		expect(mockFetch).toHaveBeenCalledWith('/api/clients');
	});
});

describe('commitClientImport', () => {
	it('calls createClient for each row', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const rows = [
			{ uuid: 'u1', name: 'Alice', email: 'a@a.com', phone: '111', address: 'Addr A', metadata: '', payer_name: '' },
			{ uuid: 'u2', name: 'Bob', email: 'b@b.com', phone: '222', address: 'Addr B', metadata: '', payer_name: '' }
		];
		await commitClientImport(rows, { clients: { createClient } as any });
		expect(createClient).toHaveBeenCalledTimes(2);
		expect(createClient).toHaveBeenCalledWith({
			uuid: 'u1', name: 'Alice', email: 'a@a.com', phone: '111', address: 'Addr A'
		});
	});

	it('trims whitespace from row fields', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const rows = [{
			uuid: '  uuid-1  ', name: '  Alice  ', email: '  a@a.com  ',
			phone: '  111  ', address: '  Main St  ', metadata: '', payer_name: ''
		}];
		await commitClientImport(rows, { clients: { createClient } as any });
		expect(createClient).toHaveBeenCalledWith({
			uuid: 'uuid-1', name: 'Alice', email: 'a@a.com', phone: '111', address: 'Main St'
		});
	});

	it('omits uuid when empty', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const rows = [{ uuid: '', name: 'Alice', email: '', phone: '', address: '', metadata: '', payer_name: '' }];
		await commitClientImport(rows, { clients: { createClient } as any });
		expect(createClient).toHaveBeenCalledWith(expect.not.objectContaining({ uuid: expect.anything() }));
	});

	it('handles empty rows array without calling createClient', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		await commitClientImport([], { clients: { createClient } as any });
		expect(createClient).not.toHaveBeenCalled();
	});
});
