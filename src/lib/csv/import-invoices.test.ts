import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./parse.js', () => ({
	parseCsvFile: vi.fn(),
	validateRequiredField: vi.fn(),
	validateNumeric: vi.fn(),
	validateDate: vi.fn(),
	validateStatus: vi.fn()
}));

import { parseCsvFile, validateRequiredField, validateNumeric, validateDate, validateStatus } from './parse.js';
import { parseInvoicesCsv, commitInvoiceImport } from './import-invoices.js';
import type { ParsedInvoiceGroup } from './types.js';

const mockParseCsvFile = vi.mocked(parseCsvFile);
const mockValidateRequiredField = vi.mocked(validateRequiredField);
const mockValidateNumeric = vi.mocked(validateNumeric);
const mockValidateDate = vi.mocked(validateDate);
const mockValidateStatus = vi.mocked(validateStatus);
const mockFetch = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
	mockValidateRequiredField.mockReturnValue(null);
	mockValidateNumeric.mockReturnValue(null);
	mockValidateDate.mockReturnValue(null);
	mockValidateStatus.mockReturnValue(null);
});

function makeFile(name = 'invoices.csv'): File {
	return new File([''], name, { type: 'text/csv' });
}

function makeInvoiceRow(overrides: Record<string, string> = {}) {
	return {
		invoice_uuid: 'uuid-1', invoice_number: 'INV-001', client_name: 'Alice',
		client_email: 'alice@example.com', date: '2024-01-15', due_date: '2024-02-15',
		tax_rate: '10', notes: 'Test note', status: 'draft', currency_code: 'USD',
		line_description: 'Consulting', line_quantity: '2', line_rate: '100',
		line_amount: '200', line_sort_order: '0', line_notes: '',
		business_snapshot: '{}', client_snapshot: '{}', payer_snapshot: '{}',
		...overrides
	};
}

describe('parseInvoicesCsv', () => {
	it('returns empty result for empty data', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.validRows).toEqual([]);
		expect(result.errors).toEqual([]);
		expect(result.groups).toEqual([]);
		expect(result.totalRows).toBe(0);
		expect(result.skippedDuplicates).toBe(0);
		expect(result.newClientsToCreate).toEqual([]);
	});

	it('groups rows by invoice_uuid', async () => {
		const rows = [
			makeInvoiceRow({ invoice_uuid: 'uuid-1', line_description: 'Line 1' }),
			makeInvoiceRow({ invoice_uuid: 'uuid-1', line_description: 'Line 2' })
		];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.groups).toHaveLength(1);
		expect(result.groups[0].lineItems).toHaveLength(2);
	});

	it('groups rows by invoice_number when no uuid', async () => {
		const rows = [
			makeInvoiceRow({ invoice_uuid: '', invoice_number: 'INV-001', line_description: 'Line A' }),
			makeInvoiceRow({ invoice_uuid: '', invoice_number: 'INV-001', line_description: 'Line B' }),
			makeInvoiceRow({ invoice_uuid: '', invoice_number: 'INV-002', line_description: 'Line C' })
		];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.groups).toHaveLength(2);
	});

	it('skips groups whose UUID already exists', async () => {
		const rows = [makeInvoiceRow({ invoice_uuid: 'existing-uuid' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([{ uuid: 'existing-uuid' }]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.skippedDuplicates).toBe(1);
		expect(result.groups).toHaveLength(0);
	});

	it('tracks new clients not in existing client list', async () => {
		const rows = [makeInvoiceRow({ client_name: 'New Client' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Existing Client' }]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.newClientsToCreate).toContain('New Client');
	});

	it('does not add existing client to newClientsToCreate', async () => {
		const rows = [makeInvoiceRow({ client_name: 'Alice' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Alice' }]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.newClientsToCreate).not.toContain('Alice');
	});

	it('collects validation errors and excludes invalid rows', async () => {
		const rows = [makeInvoiceRow({ invoice_number: '' })];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockValidateRequiredField
			.mockReturnValueOnce({ row: 1, field: 'invoice_number', message: 'invoice_number is required' })
			.mockReturnValue(null);
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.errors).toHaveLength(1);
		expect(result.groups).toHaveLength(0);
	});

	it('sets default values for missing optional group fields', async () => {
		const rows = [makeInvoiceRow({
			invoice_uuid: '', invoice_number: 'INV-999', status: '', currency_code: '',
			business_snapshot: '', client_snapshot: '', payer_snapshot: '', due_date: ''
		})];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		expect(result.groups).toHaveLength(1);
		const g = result.groups[0];
		expect(g.status).toBe('draft');
		expect(g.currencyCode).toBe('USD');
		expect(g.businessSnapshot).toBe('{}');
		expect(g.dueDate).toBe(g.date);
	});

	it('maps line item quantities and rates correctly', async () => {
		const rows = [makeInvoiceRow({
			line_quantity: '3', line_rate: '75.50', line_amount: '226.50',
			line_sort_order: '1', line_notes: 'special'
		})];
		mockParseCsvFile.mockResolvedValue({ data: rows, errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		const result = await parseInvoicesCsv(makeFile());
		const lineItem = result.groups[0].lineItems[0];
		expect(lineItem.quantity).toBe(3);
		expect(lineItem.rate).toBe(75.5);
		expect(lineItem.amount).toBe(226.5);
		expect(lineItem.sortOrder).toBe(1);
		expect(lineItem.notes).toBe('special');
	});

	it('fetches both /api/invoices and /api/clients', async () => {
		mockParseCsvFile.mockResolvedValue({ data: [], errors: [] });
		mockFetch
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) })
			.mockResolvedValueOnce({ json: vi.fn().mockResolvedValue([]) });
		await parseInvoicesCsv(makeFile());
		expect(mockFetch).toHaveBeenCalledWith('/api/invoices');
		expect(mockFetch).toHaveBeenCalledWith('/api/clients');
	});
});

describe('commitInvoiceImport', () => {
	const makeGroup = (overrides: Partial<ParsedInvoiceGroup> = {}): ParsedInvoiceGroup => ({
		invoiceUuid: 'uuid-1', invoiceNumber: 'INV-001', clientName: 'Alice',
		clientEmail: 'alice@example.com', date: '2024-01-15', dueDate: '2024-02-15',
		taxRate: 10, notes: 'Test', status: 'draft', currencyCode: 'USD',
		businessSnapshot: '{}', clientSnapshot: '{}', payerSnapshot: '{}',
		lineItems: [{ description: 'Service', quantity: 2, rate: 100, amount: 200, sortOrder: 0, notes: '' }],
		isNew: true, ...overrides
	});

	it('creates new clients before importing invoices', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createInvoice = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Alice' }]) });
		await commitInvoiceImport([makeGroup()], ['New Client'], {
			invoices: { createInvoice } as any, clients: { createClient } as any
		});
		expect(createClient).toHaveBeenCalledWith({ name: 'New Client' });
	});

	it('creates invoice with calculated totals', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createInvoice = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 5, name: 'Alice' }]) });
		const group = makeGroup({
			taxRate: 10,
			lineItems: [
				{ description: 'A', quantity: 1, rate: 100, amount: 100, sortOrder: 0, notes: '' },
				{ description: 'B', quantity: 2, rate: 50, amount: 100, sortOrder: 1, notes: '' }
			]
		});
		await commitInvoiceImport([group], [], {
			invoices: { createInvoice } as any, clients: { createClient } as any
		});
		expect(createInvoice).toHaveBeenCalledWith(
			expect.objectContaining({ subtotal: 200, tax_amount: 20, total: 220, client_id: 5 }),
			expect.any(Array)
		);
	});

	it('skips group when client is not found', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createInvoice = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'DifferentClient' }]) });
		await commitInvoiceImport([makeGroup({ clientName: 'UnknownClient' })], [], {
			invoices: { createInvoice } as any, clients: { createClient } as any
		});
		expect(createInvoice).not.toHaveBeenCalled();
	});

	it('handles empty groups array', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createInvoice = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await commitInvoiceImport([], [], {
			invoices: { createInvoice } as any, clients: { createClient } as any
		});
		expect(createInvoice).not.toHaveBeenCalled();
	});

	it('maps line items correctly to createInvoice', async () => {
		const createClient = vi.fn().mockResolvedValue(1);
		const createInvoice = vi.fn().mockResolvedValue(1);
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([{ id: 1, name: 'Alice' }]) });
		const group = makeGroup({
			lineItems: [{ description: 'Work', quantity: 3, rate: 50, amount: 150, sortOrder: 0, notes: 'note' }]
		});
		await commitInvoiceImport([group], [], {
			invoices: { createInvoice } as any, clients: { createClient } as any
		});
		expect(createInvoice).toHaveBeenCalledWith(
			expect.any(Object),
			[{ description: 'Work', quantity: 3, rate: 50, amount: 150, sort_order: 0, notes: 'note' }]
		);
	});
});
