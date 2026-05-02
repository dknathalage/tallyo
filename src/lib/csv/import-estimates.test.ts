import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./parse.js', () => ({
	parseCsvFile: vi.fn(),
	validateRequiredField: vi.fn(),
	validateNumeric: vi.fn(),
	validateDate: vi.fn(),
	validateStatus: vi.fn()
}));

import { parseCsvFile, validateRequiredField, validateNumeric, validateDate } from './parse.js';
import { parseEstimatesCsv, commitEstimateImport } from './import-estimates.js';
import type { ParsedEstimateGroup } from './types.js';

const mockParseCsvFile = vi.mocked(parseCsvFile);
const mockValidateRequiredField = vi.mocked(validateRequiredField);
const mockValidateNumeric = vi.mocked(validateNumeric);
const mockValidateDate = vi.mocked(validateDate);
const mockFetch = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
	mockValidateRequiredField.mockReturnValue(null);
	mockValidateNumeric.mockReturnValue(null);
	mockValidateDate.mockReturnValue(null);
});

function makeFile(name = 'estimates.csv'): File {
	return new File([''], name, { type: 'text/csv' });
}

function makeEstimateRow(overrides: Record<string, string> = {}) {
	return {
		estimate_uuid: 'est-uuid-1', estimate_number: 'EST-001', client_name: 'Bob',
		client_email: 'bob@example.com', date: '2024-03-01', valid_until: '2024-03-31',
		tax_rate: '5', notes: 'Estimate note', status: 'draft', currency_code: 'USD',
		line_description: 'Design Work', line_quantity: '10', line_rate: '80',
		line_amount: '800', line_sort_order: '0', line_notes: '',
		business_snapshot: '{}', client_snapshot: '{}', payer_snapshot: '{}',
		...overrides
	};
}

describe('parseEstimatesCsv', () => {
	it('returns empty result for empty data', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.validRows).toEqual([]);
		expect(result.errors).toEqual([]);
		expect(result.groups).toEqual([]);
		expect(result.totalRows).toBe(0);
		expect(result.skippedDuplicates).toBe(0);
		expect(result.newClientsToCreate).toEqual([]);
	});

	it('groups rows by estimate_uuid', async () => {
		const rows = [
			makeEstimateRow({ estimate_uuid: 'est-1', line_description: 'Line 1' }),
			makeEstimateRow({ estimate_uuid: 'est-1', line_description: 'Line 2' })
		];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.groups).toHaveLength(1);
		expect(result.groups[0]!.lineItems).toHaveLength(2);
	});

	it('groups by estimate_number when uuid is empty', async () => {
		const rows = [
			makeEstimateRow({ estimate_uuid: '', estimate_number: 'EST-001', line_description: 'A' }),
			makeEstimateRow({ estimate_uuid: '', estimate_number: 'EST-001', line_description: 'B' }),
			makeEstimateRow({ estimate_uuid: '', estimate_number: 'EST-002', line_description: 'C' })
		];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.groups).toHaveLength(2);
	});

	it('skips groups whose UUID already exists', async () => {
		const rows = [makeEstimateRow({ estimate_uuid: 'existing-uuid' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([{ uuid: 'existing-uuid' }]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.skippedDuplicates).toBe(1);
		expect(result.groups).toHaveLength(0);
	});

	it('tracks new clients not in existing client list', async () => {
		const rows = [makeEstimateRow({ client_name: 'Brand New Client' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Other Client' }]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.newClientsToCreate).toContain('Brand New Client');
	});

	it('does not add existing client to newClientsToCreate', async () => {
		const rows = [makeEstimateRow({ client_name: 'Bob' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Bob' }]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.newClientsToCreate).not.toContain('Bob');
	});

	it('collects validation errors and excludes invalid rows', async () => {
		const rows = [makeEstimateRow({ estimate_number: '' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockValidateRequiredField
			.mockReturnValueOnce({ row: 1, field: 'estimate_number', message: 'estimate_number is required' })
			.mockReturnValue(null);
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.errors).toHaveLength(1);
		expect(result.groups).toHaveLength(0);
	});

	it('applies default values for missing optional fields', async () => {
		const rows = [makeEstimateRow({
			estimate_uuid: '', estimate_number: 'EST-100', status: '',
			currency_code: '', business_snapshot: '', client_snapshot: '',
			payer_snapshot: '', valid_until: ''
		})];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		const g = result.groups[0]!;
		expect(g.status).toBe('draft');
		expect(g.currencyCode).toBe('USD');
		expect(g.businessSnapshot).toBe('{}');
		expect(g.validUntil).toBe(g.date);
	});

	it('validates estimate status accepted as valid', async () => {
		const rows = [makeEstimateRow({ status: 'accepted' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.errors).toHaveLength(0);
		expect(result.groups[0]!.status).toBe('accepted');
	});

	it('rejects invalid estimate status', async () => {
		const rows = [makeEstimateRow({ status: 'cancelled' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.errors.some((e) => e.field === 'status')).toBe(true);
		expect(result.groups).toHaveLength(0);
	});

	it('fetches both /api/estimates and /api/clients', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		await parseEstimatesCsv(makeFile());
		expect(mockFetch).toHaveBeenCalledWith('/api/estimates?limit=10000');
		expect(mockFetch).toHaveBeenCalledWith('/api/clients?limit=10000');
	});

	it('sets isNew to true for all new groups', async () => {
		const rows = [makeEstimateRow()];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseEstimatesCsv(makeFile());
		expect(result.groups[0]!.isNew).toBe(true);
	});
});

describe('commitEstimateImport', () => {
	const makeGroup = (overrides: Partial<ParsedEstimateGroup> = {}): ParsedEstimateGroup => ({
		estimateUuid: 'est-uuid-1', estimateNumber: 'EST-001', clientName: 'Bob',
		clientEmail: 'bob@example.com', date: '2024-03-01', validUntil: '2024-03-31',
		taxRate: 5, notes: 'Note', status: 'draft', currencyCode: 'USD',
		businessSnapshot: '{}', clientSnapshot: '{}', payerSnapshot: '{}',
		lineItems: [{ description: 'Design', quantity: 10, rate: 80, amount: 800, sortOrder: 0, notes: '' }],
		isNew: true, ...overrides
	});

	it('creates new clients before importing estimates', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createEstimate = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Bob' }]) });
		await commitEstimateImport([makeGroup()], ['New Client'], {
			estimates: { createEstimate } as any, clients: { createClient } as any
		});
		expect(createClient).toHaveBeenCalledWith({ name: 'New Client' });
	});

	it('creates estimate with calculated subtotal, taxAmount, total', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createEstimate = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 7, name: 'Bob' }]) });
		const group = makeGroup({
			taxRate: 10,
			lineItems: [
				{ description: 'Task 1', quantity: 1, rate: 200, amount: 200, sortOrder: 0, notes: '' },
				{ description: 'Task 2', quantity: 2, rate: 50, amount: 100, sortOrder: 1, notes: '' }
			]
		});
		await commitEstimateImport([group], [], {
			estimates: { createEstimate } as any, clients: { createClient } as any
		});
		expect(createEstimate).toHaveBeenCalledWith(
			expect.objectContaining({ subtotal: 300, tax_amount: 30, total: 330, client_id: 7 }),
			expect.any(Array)
		);
	});

	it('skips group when client is not found', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createEstimate = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'SomeoneElse' }]) });
		await commitEstimateImport([makeGroup({ clientName: 'UnknownClient' })], [], {
			estimates: { createEstimate } as any, clients: { createClient } as any
		});
		expect(createEstimate).not.toHaveBeenCalled();
	});

	it('handles empty groups without creating any estimates', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createEstimate = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await commitEstimateImport([], [], {
			estimates: { createEstimate } as any, clients: { createClient } as any
		});
		expect(createEstimate).not.toHaveBeenCalled();
	});

	it('maps line items correctly', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createEstimate = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Bob' }]) });
		const group = makeGroup({
			lineItems: [{ description: 'Task', quantity: 5, rate: 30, amount: 150, sortOrder: 2, notes: 'a note' }]
		});
		await commitEstimateImport([group], [], {
			estimates: { createEstimate } as any, clients: { createClient } as any
		});
		expect(createEstimate).toHaveBeenCalledWith(
			expect.any(Object),
			[{ description: 'Task', quantity: 5, rate: 30, amount: 150, sort_order: 2, notes: 'a note' }]
		);
	});
});
