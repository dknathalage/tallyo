import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockFrom = vi.fn().mockReturnThis();
const mockWhere = vi.fn();
const mockSelect = vi.fn().mockReturnValue({ from: mockFrom });

vi.mock('../db/connection.js', () => ({
	getDb: vi.fn(() => ({
		select: mockSelect
	}))
}));

import { generateInvoiceNumber } from './invoice-number.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockSelect.mockReturnValue({ from: mockFrom });
	mockFrom.mockReturnValue({ where: mockWhere });
});

describe('generateInvoiceNumber', () => {
	it('is an async function', () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		expect(generateInvoiceNumber()).toBeInstanceOf(Promise);
	});

	it('returns INV-0001 when no invoices exist (null result)', async () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		expect(await generateInvoiceNumber()).toBe('INV-0001');
	});

	it('returns INV-0001 when query returns empty array', async () => {
		mockWhere.mockResolvedValue([]);
		expect(await generateInvoiceNumber()).toBe('INV-0001');
	});

	it('increments from the current max invoice number', async () => {
		mockWhere.mockResolvedValue([{ max_num: 5 }]);
		expect(await generateInvoiceNumber()).toBe('INV-0006');
	});

	it('pads the number to 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 9 }]);
		expect(await generateInvoiceNumber()).toBe('INV-0010');
	});

	it('handles large invoice numbers beyond 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 9999 }]);
		expect(await generateInvoiceNumber()).toBe('INV-10000');
	});

	it('returns INV-0001 when only non-standard invoice numbers exist', async () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		expect(await generateInvoiceNumber()).toBe('INV-0001');
	});

	it('returns INV-0001 when max_num is 0', async () => {
		mockWhere.mockResolvedValue([{ max_num: 0 }]);
		expect(await generateInvoiceNumber()).toBe('INV-0001');
	});
});
