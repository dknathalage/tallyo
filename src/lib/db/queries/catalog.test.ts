import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('../audit.js', () => ({
	logAudit: vi.fn(),
	computeChanges: vi.fn().mockReturnValue({})
}));

import {
	getCatalogItems,
	searchCatalogItems,
	createCatalogItem,
	updateCatalogItem,
	deleteCatalogItem
} from './catalog.js';
import { query, execute, save } from '../connection.svelte.js';
import { logAudit } from '../audit.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);
const mockLogAudit = vi.mocked(logAudit);

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

	it('returns empty array when catalog is empty', () => {
		mockQuery.mockReturnValue([]);

		const result = getCatalogItems();

		expect(result).toEqual([]);
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
		expect(mockSave).toHaveBeenCalled();
	});

	it('defaults rate, unit, category, sku to empty/zero when not provided', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createCatalogItem({ name: 'Basic Item' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO catalog_items'),
			expect.arrayContaining(['Basic Item', 0, '', '', ''])
		);
	});

	it('audit logs the creation with name and rate changes', async () => {
		mockQuery.mockReturnValue([{ id: 7 }]);

		await createCatalogItem({ name: 'Audit Item', rate: 50 });

		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'catalog',
				entity_id: 7,
				action: 'create'
			})
		);
	});

	it('throws when name is empty', async () => {
		await expect(createCatalogItem({ name: '' })).rejects.toThrow(
			'Catalog item name is required'
		);
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is only whitespace', async () => {
		await expect(createCatalogItem({ name: '   ' })).rejects.toThrow(
			'Catalog item name is required'
		);
	});
});

describe('updateCatalogItem', () => {
	it('updates the catalog item fields', async () => {
		// getCatalogItem called first: return existing item
		mockQuery.mockReturnValue([{ id: 2, name: 'Old Name', rate: 10 }]);

		await updateCatalogItem(2, { name: 'New Name', rate: 20 });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE catalog_items SET'),
			expect.arrayContaining(['New Name', 20, 2])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('throws when new name is empty', async () => {
		await expect(updateCatalogItem(1, { name: '' })).rejects.toThrow(
			'Catalog item name is required'
		);
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('logs an audit entry when fields change', async () => {
		const { computeChanges } = await import('../audit.js');
		const mockComputeChanges = vi.mocked(computeChanges);
		mockComputeChanges.mockReturnValue({ name: { old: 'Old', new: 'New' } });

		mockQuery.mockReturnValue([{ id: 2, name: 'Old', rate: 10 }]);

		await updateCatalogItem(2, { name: 'New', rate: 10 });

		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'catalog',
				entity_id: 2,
				action: 'update'
			})
		);
	});

	it('does not log an audit entry when nothing changed', async () => {
		const { computeChanges } = await import('../audit.js');
		vi.mocked(computeChanges).mockReturnValue({});

		mockQuery.mockReturnValue([{ id: 2, name: 'Same', rate: 10 }]);

		await updateCatalogItem(2, { name: 'Same', rate: 10 });

		expect(mockLogAudit).not.toHaveBeenCalled();
	});
});

describe('deleteCatalogItem', () => {
	it('deletes the item and saves', async () => {
		mockQuery.mockReturnValue([{ id: 5, name: 'Doomed Item' }]);

		await deleteCatalogItem(5);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM catalog_items WHERE id = ?', [5]);
		expect(mockSave).toHaveBeenCalled();
	});

	it('audit logs the deletion with the item name as context', async () => {
		mockQuery.mockReturnValue([{ id: 5, name: 'Doomed Item' }]);

		await deleteCatalogItem(5);

		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'catalog',
				entity_id: 5,
				action: 'delete',
				context: 'Doomed Item'
			})
		);
	});

	it('audit logs with empty context when item not found', async () => {
		mockQuery.mockReturnValue([]);

		await deleteCatalogItem(999);

		expect(mockLogAudit).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'catalog',
				entity_id: 999,
				action: 'delete',
				context: ''
			})
		);
	});
});
