import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn()
}));

import { generateInvoiceNumber, generateEstimateNumber } from './number-generators.js';
import { query } from './connection.js';

const mockQuery = vi.mocked(query);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('generateInvoiceNumber', () => {
	it('returns INV-0001 when no invoices exist', () => {
		mockQuery.mockReturnValue([{ max_num: null }]);

		const result = generateInvoiceNumber();

		expect(result).toBe('INV-0001');
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("GLOB 'INV-[0-9]*'")
		);
	});

	it('returns INV-0001 when max_num is 0', () => {
		mockQuery.mockReturnValue([{ max_num: 0 }]);

		const result = generateInvoiceNumber();

		expect(result).toBe('INV-0001');
	});

	it('increments existing invoice number', () => {
		mockQuery.mockReturnValue([{ max_num: 5 }]);

		const result = generateInvoiceNumber();

		expect(result).toBe('INV-0006');
	});

	it('pads to 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 42 }]);

		const result = generateInvoiceNumber();

		expect(result).toBe('INV-0043');
	});

	it('handles large numbers beyond 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 99999 }]);

		const result = generateInvoiceNumber();

		expect(result).toBe('INV-100000');
	});

	it('returns INV-0001 when query returns empty array', () => {
		mockQuery.mockReturnValue([]);

		const result = generateInvoiceNumber();

		expect(result).toBe('INV-0001');
	});
});

describe('generateEstimateNumber', () => {
	it('returns EST-0001 when no estimates exist', () => {
		mockQuery.mockReturnValue([{ max_num: null }]);

		const result = generateEstimateNumber();

		expect(result).toBe('EST-0001');
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining("GLOB 'EST-[0-9]*'")
		);
	});

	it('returns EST-0001 when max_num is 0', () => {
		mockQuery.mockReturnValue([{ max_num: 0 }]);

		const result = generateEstimateNumber();

		expect(result).toBe('EST-0001');
	});

	it('increments existing estimate number', () => {
		mockQuery.mockReturnValue([{ max_num: 10 }]);

		const result = generateEstimateNumber();

		expect(result).toBe('EST-0011');
	});

	it('pads to 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 3 }]);

		const result = generateEstimateNumber();

		expect(result).toBe('EST-0004');
	});

	it('handles large numbers beyond 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 12345 }]);

		const result = generateEstimateNumber();

		expect(result).toBe('EST-12346');
	});

	it('returns EST-0001 when query returns empty array', () => {
		mockQuery.mockReturnValue([]);

		const result = generateEstimateNumber();

		expect(result).toBe('EST-0001');
	});
});
