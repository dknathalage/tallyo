import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

import {
	getRateTiers,
	createRateTier,
	updateRateTier,
	deleteRateTier,
	getDefaultTier
} from './rate-tiers.js';
import { query, execute, save } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getRateTiers', () => {
	it('returns all tiers ordered by sort_order then name', () => {
		const tiers = [
			{ id: 1, name: 'Standard', sort_order: 0 },
			{ id: 2, name: 'Premium', sort_order: 1 }
		];
		mockQuery.mockReturnValue(tiers);

		const result = getRateTiers();

		expect(result).toEqual(tiers);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('ORDER BY sort_order, name')
		);
	});

	it('returns empty array when no tiers exist', () => {
		mockQuery.mockReturnValue([]);

		const result = getRateTiers();

		expect(result).toEqual([]);
	});
});

describe('getDefaultTier', () => {
	it('returns the first tier (lowest sort_order) as default', () => {
		const tier = { id: 1, name: 'Standard', sort_order: 0 };
		mockQuery.mockReturnValue([tier]);

		const result = getDefaultTier();

		expect(result).toEqual(tier);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('ORDER BY sort_order, id LIMIT 1')
		);
	});

	it('returns null when no tiers exist', () => {
		mockQuery.mockReturnValue([]);

		const result = getDefaultTier();

		expect(result).toBeNull();
	});
});

describe('createRateTier', () => {
	it('inserts a tier and returns its id', async () => {
		mockQuery.mockReturnValue([{ id: 4 }]);

		const id = await createRateTier({ name: 'VIP', description: 'VIP clients', sort_order: 2 });

		expect(id).toBe(4);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO rate_tiers'),
			expect.arrayContaining(['VIP', 'VIP clients', 2])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('defaults description and sort_order when not provided', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createRateTier({ name: 'Basic' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO rate_tiers'),
			expect.arrayContaining(['Basic', '', 0])
		);
	});

	it('inserts a uuid as first parameter', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createRateTier({ name: 'Standard' });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(typeof args[0]).toBe('string');
		expect(args[0]).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
		);
	});

	it('throws when name is empty', async () => {
		await expect(createRateTier({ name: '' })).rejects.toThrow('Tier name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is only whitespace', async () => {
		await expect(createRateTier({ name: '   ' })).rejects.toThrow('Tier name is required');
	});
});

describe('updateRateTier', () => {
	it('updates tier fields', async () => {
		await updateRateTier(2, { name: 'Premium Plus', description: 'Top tier', sort_order: 3 });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE rate_tiers SET name = ?'),
			expect.arrayContaining(['Premium Plus', 'Top tier', 3, 2])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('defaults description and sort_order when not provided', async () => {
		await updateRateTier(1, { name: 'Standard' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE rate_tiers SET name = ?'),
			expect.arrayContaining(['Standard', '', 0, 1])
		);
	});

	it('throws when new name is empty', async () => {
		await expect(updateRateTier(1, { name: '' })).rejects.toThrow('Tier name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});
});

describe('deleteRateTier', () => {
	it('deletes a tier when more than one tier exists', async () => {
		mockQuery.mockReturnValue([
			{ id: 1, name: 'Standard' },
			{ id: 2, name: 'Premium' }
		]);

		await deleteRateTier(2);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM rate_tiers WHERE id = ?', [2]);
		expect(mockSave).toHaveBeenCalled();
	});

	it('throws when trying to delete the last tier', async () => {
		mockQuery.mockReturnValue([{ id: 1, name: 'Standard' }]);

		await expect(deleteRateTier(1)).rejects.toThrow('Cannot delete the last tier');
		expect(mockExecute).not.toHaveBeenCalled();
	});
});
