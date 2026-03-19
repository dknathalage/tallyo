import { describe, it, expect } from 'vitest';
import { diffCatalog } from './diff-catalog.js';
import type { MappedRow } from './map-columns.js';

function makeMappedRow(overrides: Partial<MappedRow> = {}): MappedRow {
	return {
		name: 'Test Item', sku: 'SKU-001', unit: 'hr', category: 'Services',
		rate: 100, tierRates: {}, metadata: {}, _raw: {}, _errors: [], ...overrides
	};
}

function makeExistingItem(overrides = {}) {
	return { id: 1, name: 'Test Item', sku: 'SKU-001', rate: 100, unit: 'hr', category: 'Services', ...overrides };
}

describe('diffCatalog', () => {
	describe('empty inputs', () => {
		it('returns empty diff for empty inputs', () => {
			const result = diffCatalog([], []);
			expect(result.newItems).toEqual([]);
			expect(result.updatedItems).toEqual([]);
			expect(result.unchangedCount).toBe(0);
			expect(result.errorItems).toEqual([]);
			expect(result.summary).toEqual({ total: 0, new: 0, updated: 0, unchanged: 0, errors: 0 });
		});

		it('all rows become new items when no existing items', () => {
			const rows = [makeMappedRow({ sku: 'A' }), makeMappedRow({ sku: 'B' })];
			const result = diffCatalog(rows, []);
			expect(result.newItems).toHaveLength(2);
			expect(result.summary.new).toBe(2);
		});
	});

	describe('new items', () => {
		it('treats row as new when SKU not in existing items', () => {
			const rows = [makeMappedRow({ sku: 'NEW-SKU' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'OTHER-SKU' })]);
			expect(result.newItems).toHaveLength(1);
			expect(result.newItems[0].sku).toBe('NEW-SKU');
		});

		it('treats row as new when SKU is empty', () => {
			const rows = [makeMappedRow({ sku: '' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'SOMETHING' })]);
			expect(result.newItems).toHaveLength(1);
		});
	});

	describe('updated items', () => {
		it('detects name change', () => {
			const rows = [makeMappedRow({ sku: 'SKU-001', name: 'New Name' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'SKU-001', name: 'Old Name' })]);
			expect(result.updatedItems).toHaveLength(1);
			expect(result.updatedItems[0].changes).toContainEqual(expect.stringContaining('Name:'));
		});

		it('detects rate change', () => {
			const rows = [makeMappedRow({ sku: 'SKU-001', rate: 200 })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'SKU-001', rate: 100 })]);
			expect(result.updatedItems).toHaveLength(1);
			expect(result.updatedItems[0].changes).toContainEqual(expect.stringContaining('Rate:'));
		});

		it('detects unit change', () => {
			const rows = [makeMappedRow({ sku: 'SKU-001', unit: 'ea' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'SKU-001', unit: 'hr' })]);
			expect(result.updatedItems).toHaveLength(1);
			expect(result.updatedItems[0].changes).toContainEqual(expect.stringContaining('Unit:'));
		});

		it('detects category change', () => {
			const rows = [makeMappedRow({ sku: 'SKU-001', category: 'New Category' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'SKU-001', category: 'Services' })]);
			expect(result.updatedItems).toHaveLength(1);
			expect(result.updatedItems[0].changes).toContainEqual(expect.stringContaining('Category:'));
		});

		it('detects multiple changes at once', () => {
			const rows = [makeMappedRow({ sku: 'SKU-001', name: 'New Name', rate: 999, unit: 'day' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: 'SKU-001', name: 'Old Name', rate: 100, unit: 'hr' })]);
			expect(result.updatedItems[0].changes).toHaveLength(3);
		});

		it('includes existing and incoming in updated item', () => {
			const row = makeMappedRow({ sku: 'SKU-001', name: 'Updated' });
			const existing = makeExistingItem({ sku: 'SKU-001', name: 'Original' });
			const result = diffCatalog([row], [existing]);
			expect(result.updatedItems[0].existing).toEqual(existing);
			expect(result.updatedItems[0].incoming).toEqual(row);
		});
	});

	describe('unchanged items', () => {
		it('counts unchanged when row matches existing exactly', () => {
			const rows = [makeMappedRow({ sku: 'SKU-001', name: 'Test Item', rate: 100, unit: 'hr', category: 'Services' })];
			const result = diffCatalog(rows, [makeExistingItem()]);
			expect(result.unchangedCount).toBe(1);
			expect(result.updatedItems).toHaveLength(0);
		});
	});

	describe('error items', () => {
		it('routes rows with _errors to errorItems', () => {
			const rows = [makeMappedRow({ _errors: ['Name is required'] })];
			const result = diffCatalog(rows, []);
			expect(result.errorItems).toHaveLength(1);
			expect(result.newItems).toHaveLength(0);
		});

		it('does not include error items in new/updated/unchanged counts', () => {
			const rows = [
				makeMappedRow({ sku: 'GOOD', _errors: [] }),
				makeMappedRow({ _errors: ['bad'] })
			];
			const result = diffCatalog(rows, []);
			expect(result.newItems).toHaveLength(1);
			expect(result.errorItems).toHaveLength(1);
			expect(result.summary.errors).toBe(1);
		});
	});

	describe('SKU matching', () => {
		it('matches SKU case-insensitively', () => {
			const rows = [makeMappedRow({ sku: 'sku-001' })];
			const existing = [makeExistingItem({ sku: 'SKU-001' })];
			const result = diffCatalog(rows, existing);
			// Should match by SKU (not be treated as new), result is either unchanged or updated
			expect(result.newItems).toHaveLength(0);
			expect(result.unchangedCount + result.updatedItems.length).toBe(1);
		});

		it('ignores existing items with empty SKU for lookup', () => {
			const rows = [makeMappedRow({ sku: '' })];
			const result = diffCatalog(rows, [makeExistingItem({ sku: '' })]);
			expect(result.newItems).toHaveLength(1);
		});

		it('trims whitespace from SKU when matching', () => {
			const rows = [makeMappedRow({ sku: '  SKU-001  ' })];
			const existing = [makeExistingItem({ sku: 'SKU-001' })];
			const result = diffCatalog(rows, existing);
			// Should match by SKU (not be treated as new)
			expect(result.newItems).toHaveLength(0);
			expect(result.unchangedCount + result.updatedItems.length).toBe(1);
		});
	});

	describe('summary', () => {
		it('computes correct summary totals', () => {
			const rows = [
				makeMappedRow({ sku: 'NEW-1' }),
				makeMappedRow({ sku: 'UPD-1', name: 'Changed' }),
				makeMappedRow({ sku: 'SAME-1', name: 'Same', rate: 50, unit: 'hr', category: 'C' }),
				makeMappedRow({ _errors: ['error'] })
			];
			const existing = [
				makeExistingItem({ sku: 'UPD-1', name: 'Original' }),
				makeExistingItem({ id: 2, sku: 'SAME-1', name: 'Same', rate: 50, unit: 'hr', category: 'C' })
			];
			const result = diffCatalog(rows, existing);
			expect(result.summary.total).toBe(4);
			expect(result.summary.new).toBe(1);
			expect(result.summary.updated).toBe(1);
			expect(result.summary.unchanged).toBe(1);
			expect(result.summary.errors).toBe(1);
		});
	});
});
