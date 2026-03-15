import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

import {
	getTaxRates,
	getDefaultTaxRate,
	createTaxRate,
	updateTaxRate,
	deleteTaxRate
} from './tax-rates.js';
import { query, execute, save } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getTaxRates', () => {
	it('returns all tax rates ordered by default first, then name', () => {
		const rates = [
			{ id: 1, name: 'GST', rate: 10, is_default: 1 },
			{ id: 2, name: 'VAT', rate: 20, is_default: 0 }
		];
		mockQuery.mockReturnValue(rates);

		const result = getTaxRates();

		expect(result).toEqual(rates);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('ORDER BY is_default DESC, name ASC')
		);
	});

	it('returns empty array when no tax rates exist', () => {
		mockQuery.mockReturnValue([]);

		const result = getTaxRates();

		expect(result).toEqual([]);
	});

	it('returns multiple rates correctly', () => {
		const rates = [
			{ id: 1, name: 'GST', rate: 10, is_default: 1 },
			{ id: 2, name: 'Reduced', rate: 5, is_default: 0 },
			{ id: 3, name: 'Zero', rate: 0, is_default: 0 }
		];
		mockQuery.mockReturnValue(rates);

		const result = getTaxRates();

		expect(result).toHaveLength(3);
	});
});

describe('getDefaultTaxRate', () => {
	it('returns the default tax rate when one exists', () => {
		const defaultRate = { id: 1, name: 'GST', rate: 10, is_default: 1 };
		mockQuery.mockReturnValue([defaultRate]);

		const result = getDefaultTaxRate();

		expect(result).toEqual(defaultRate);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE is_default = 1 LIMIT 1')
		);
	});

	it('returns null when no default tax rate is set', () => {
		mockQuery.mockReturnValue([]);

		const result = getDefaultTaxRate();

		expect(result).toBeNull();
	});
});

describe('createTaxRate', () => {
	it('inserts a tax rate and returns its id', async () => {
		mockQuery.mockReturnValue([{ id: 2 }]);

		const id = await createTaxRate({ name: 'VAT', rate: 20 });

		expect(id).toBe(2);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO tax_rates'),
			expect.arrayContaining(['VAT', 20])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('sets is_default to 0 when not specified', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createTaxRate({ name: 'GST', rate: 10 });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO tax_rates'),
			expect.arrayContaining([0])
		);
	});

	it('clears other defaults first when is_default is true', async () => {
		mockQuery.mockReturnValue([{ id: 3 }]);

		await createTaxRate({ name: 'New Default', rate: 15, is_default: true });

		// First call should clear existing defaults
		expect(mockExecute).toHaveBeenCalledWith('UPDATE tax_rates SET is_default = 0');
		// Second call should insert the new rate as default
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO tax_rates'),
			expect.arrayContaining(['New Default', 15, 1])
		);
	});

	it('does not clear other defaults when is_default is false', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createTaxRate({ name: 'Optional', rate: 5, is_default: false });

		// Should only have the INSERT call, not the UPDATE
		expect(mockExecute).toHaveBeenCalledTimes(1);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO tax_rates'),
			expect.any(Array)
		);
	});
});

describe('updateTaxRate', () => {
	it('updates the tax rate fields', async () => {
		await updateTaxRate(1, { name: 'GST Updated', rate: 11 });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE tax_rates SET name = ?'),
			expect.arrayContaining(['GST Updated', 11, 1])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('clears other defaults before updating when is_default is true', async () => {
		await updateTaxRate(2, { name: 'New Default', rate: 10, is_default: true });

		// First call should clear defaults for other rows
		expect(mockExecute).toHaveBeenCalledWith(
			'UPDATE tax_rates SET is_default = 0 WHERE id != ?',
			[2]
		);
		// Second call is the actual update
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE tax_rates SET name = ?'),
			expect.arrayContaining(['New Default', 10, 1, 2])
		);
	});

	it('does not clear other defaults when is_default is false', async () => {
		await updateTaxRate(1, { name: 'GST', rate: 10, is_default: false });

		// Only the UPDATE call, no clearing of other defaults
		expect(mockExecute).toHaveBeenCalledTimes(1);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE tax_rates SET name = ?'),
			expect.any(Array)
		);
	});
});

describe('deleteTaxRate', () => {
	it('deletes the tax rate by id and saves', async () => {
		await deleteTaxRate(3);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM tax_rates WHERE id = ?', [3]);
		expect(mockSave).toHaveBeenCalled();
	});
});
