import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockFrom = vi.fn().mockReturnThis();
const mockWhere = vi.fn();
const mockSelect = vi.fn().mockReturnValue({ from: mockFrom });

vi.mock('../db/connection.js', () => ({
	getDb: vi.fn(() => ({
		select: mockSelect
	}))
}));

import { generateEstimateNumber } from './estimate-number.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockSelect.mockReturnValue({ from: mockFrom });
	mockFrom.mockReturnValue({ where: mockWhere });
});

describe('generateEstimateNumber', () => {
	it('is an async function', () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		expect(generateEstimateNumber()).toBeInstanceOf(Promise);
	});

	it('returns EST-0001 when no estimates exist', async () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		expect(await generateEstimateNumber()).toBe('EST-0001');
	});

	it('returns EST-0001 when query returns empty', async () => {
		mockWhere.mockResolvedValue([]);
		expect(await generateEstimateNumber()).toBe('EST-0001');
	});

	it('increments from the current max estimate number', async () => {
		mockWhere.mockResolvedValue([{ max_num: 5 }]);
		expect(await generateEstimateNumber()).toBe('EST-0006');
	});

	it('pads the number to 4 digits', async () => {
		mockWhere.mockResolvedValue([{ max_num: 9 }]);
		expect(await generateEstimateNumber()).toBe('EST-0010');
	});

	it('handles large estimate numbers', async () => {
		mockWhere.mockResolvedValue([{ max_num: 9999 }]);
		expect(await generateEstimateNumber()).toBe('EST-10000');
	});

	it('returns EST-0001 when max_num is 0', async () => {
		mockWhere.mockResolvedValue([{ max_num: 0 }]);
		expect(await generateEstimateNumber()).toBe('EST-0001');
	});

	it('never returns NaN in the result', async () => {
		mockWhere.mockResolvedValue([{ max_num: null }]);
		const result = await generateEstimateNumber();
		expect(result).not.toContain('NaN');
		expect(result).toMatch(/^EST-\d+$/);
	});
});
