import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockFrom = vi.fn().mockReturnThis();
const mockWhere = vi.fn();
const mockSelect = vi.fn().mockReturnValue({ from: mockFrom });

vi.mock('./connection.js', () => ({
	getDb: vi.fn(() => ({
		select: mockSelect
	}))
}));

import { generateInvoiceNumber, generateEstimateNumber } from './number-generators.js';

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

	it('returns INV-0001 when no invoices exist', async () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		const result = await generateInvoiceNumber();
		expect(result).toBe('INV-0001');
	});

	it('returns INV-0001 when max_num is 0', async () => {
		mockWhere.mockResolvedValue([{ max_num: 0 }]);
		const result = await generateInvoiceNumber();
		expect(result).toBe('INV-0001');
	});

	it('increments existing invoice number', async () => {
		mockWhere.mockResolvedValue([{ max_num: 5 }]);
		const result = await generateInvoiceNumber();
		expect(result).toBe('INV-0006');
	});

	it('pads to 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 42 }]);
		const result = await generateInvoiceNumber();
		expect(result).toBe('INV-0043');
	});

	it('handles large numbers beyond 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 99999 }]);
		const result = await generateInvoiceNumber();
		expect(result).toBe('INV-100000');
	});

	it('returns INV-0001 when query returns empty array', async () => {
		mockWhere.mockResolvedValue([]);
		const result = await generateInvoiceNumber();
		expect(result).toBe('INV-0001');
	});
});

describe('generateEstimateNumber', () => {
	it('is an async function', () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		expect(generateEstimateNumber()).toBeInstanceOf(Promise);
	});

	it('returns EST-0001 when no estimates exist', async () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		const result = await generateEstimateNumber();
		expect(result).toBe('EST-0001');
	});

	it('returns EST-0001 when max_num is 0', async () => {
		mockWhere.mockResolvedValue([{ max_num: 0 }]);
		const result = await generateEstimateNumber();
		expect(result).toBe('EST-0001');
	});

	it('increments existing estimate number', async () => {
		mockWhere.mockResolvedValue([{ max_num: 10 }]);
		const result = await generateEstimateNumber();
		expect(result).toBe('EST-0011');
	});

	it('pads to 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 3 }]);
		const result = await generateEstimateNumber();
		expect(result).toBe('EST-0004');
	});

	it('handles large numbers beyond 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 12345 }]);
		const result = await generateEstimateNumber();
		expect(result).toBe('EST-12346');
	});

	it('returns EST-0001 when query returns empty array', async () => {
		mockWhere.mockResolvedValue([]);
		const result = await generateEstimateNumber();
		expect(result).toBe('EST-0001');
	});
});
