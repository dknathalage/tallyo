import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn()
}));

import {
	getCatalogItems,
	searchCatalogItems,
	createCatalogItem,
	updateCatalogItem,
	deleteCatalogItem
} from './catalog.js';
import { query, execute } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getCatalogItems', () => {
	it('returns all items when no filters provided', () => {
		mockQuery.mockReturnValue([]);

		getCatalogItems();

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT * FROM catalog_items ORDER BY name'
		);
	});

	it('returns empty result when catalog is empty', () => {
		mockQuery.mockReturnValue([]);

		const result = getCatalogItems();

		expect(result.data).toEqual([]);
		expect(result.total).toBe(0);
	});

	it('filters by search term', () => {
		mockQuery.mockReturnValue([]);

		getCatalogItems('widget');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('name LIKE ? OR sku LIKE ?'),
			['%widget%', '%widget%']
		);
	});

	it('filters by category', () => {
		mockQuery.mockReturnValue([]);

		getCatalogItems(undefined, 'services');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('category = ?'),
			['services']
		);
	});

	it('filters by both search and category', () => {
		mockQuery.mockReturnValue([]);

		getCatalogItems('widget', 'hardware');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('name LIKE ? OR sku LIKE ?'),
			['%widget%', '%widget%', 'hardware']
		);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('category = ?'),
			['%widget%', '%widget%', 'hardware']
		);
	});
});

describe('searchCatalogItems', () => {
	it('searches by term and applies default limit of 10', () => {
		mockQuery.mockReturnValue([]);

		searchCatalogItems('bolt');

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('LIMIT ?'),
			['%bolt%', '%bolt%', 10]
		);
	});

	it('applies custom limit', () => {
		mockQuery.mockReturnValue([]);

		searchCatalogItems('bolt', 5);

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('LIMIT ?'),
			['%bolt%', '%bolt%', 5]
		);
	});

	it('returns empty array when no results match', () => {
		mockQuery.mockReturnValue([]);

		const result = searchCatalogItems('nonexistent-xyz');

		expect(result).toEqual([]);
	});
});

describe('createCatalogItem', () => {
	it('inserts a catalog item and returns its id', async () => {
		mockQuery.mockReturnValue([{ id: 3 }]);

		const id = await createCatalogItem({ name: 'Widget Pro', rate: 25 });

		expect(id).toBe(3);
		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO catalog_items'),
			expect.arrayContaining(['Widget Pro', 25])
		);
		// save() and logAudit() are now the repository's responsibility
	});

	it('defaults rate, unit, category, sku to empty/zero when not provided', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createCatalogItem({ name: 'Basic Item' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO catalog_items'),
			expect.arrayContaining(['Basic Item', 0, '', '', ''])
		);
	});

	it('throws when name is empty', () => {
		expect(() => createCatalogItem({ name: '' })).toThrow(
			'Catalog item name is required'
		);
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is only whitespace', () => {
		expect(() => createCatalogItem({ name: '   ' })).toThrow(
			'Catalog item name is required'
		);
	});
});

describe('updateCatalogItem', () => {
	it('updates the catalog item fields', async () => {
		mockQuery.mockReturnValue([]);

		await updateCatalogItem(2, { name: 'New Name', rate: 20 });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE catalog_items SET'),
			expect.arrayContaining(['New Name', 20, 2])
		);
		// save() and logAudit() are now the repository's responsibility
	});

	it('throws when new name is empty', () => {
		expect(() => updateCatalogItem(1, { name: '' })).toThrow(
			'Catalog item name is required'
		);
		expect(mockExecute).not.toHaveBeenCalled();
	});
});

describe('deleteCatalogItem', () => {
	it('deletes the item', async () => {
		await deleteCatalogItem(5);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM catalog_items WHERE id = ?', [5]);
		// save() and logAudit() are now the repository's responsibility
	});

	it('propagates execute errors', () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('DELETE failed');
		});

		expect(() => deleteCatalogItem(5)).toThrow('DELETE failed');
	});
});
