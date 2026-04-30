import { describe, it, expect } from 'vitest';
import { autoDetectMapping, applyMapping } from './map-columns.js';
import type { ColumnMappingConfig } from './map-columns.js';

describe('autoDetectMapping', () => {
	describe('fuzzy header matching', () => {
		it('maps "name" header to name field', () => {
			expect(autoDetectMapping(['name']).fieldMap['name']).toBe('name');
		});

		it('maps "item name" to name', () => {
			expect(autoDetectMapping(['item name']).fieldMap['item name']).toBe('name');
		});

		it('maps "sku" to sku', () => {
			expect(autoDetectMapping(['sku']).fieldMap['sku']).toBe('sku');
		});

		it('maps "code" to sku', () => {
			expect(autoDetectMapping(['code']).fieldMap['code']).toBe('sku');
		});

		it('maps "item code" to sku', () => {
			expect(autoDetectMapping(['item code']).fieldMap['item code']).toBe('sku');
		});

		it('maps "rate" to rate', () => {
			expect(autoDetectMapping(['rate']).fieldMap['rate']).toBe('rate');
		});

		it('maps "price" to rate', () => {
			expect(autoDetectMapping(['price']).fieldMap['price']).toBe('rate');
		});

		it('maps "unit" to unit', () => {
			expect(autoDetectMapping(['unit']).fieldMap['unit']).toBe('unit');
		});

		it('maps "category" to category', () => {
			expect(autoDetectMapping(['category']).fieldMap['category']).toBe('category');
		});

		it('maps "description" to name', () => {
			expect(autoDetectMapping(['description']).fieldMap['description']).toBe('name');
		});

		it('maps "unit of measure" to unit', () => {
			expect(autoDetectMapping(['unit of measure']).fieldMap['unit of measure']).toBe('unit');
		});

		it('is case-insensitive', () => {
			const result = autoDetectMapping(['Name', 'SKU', 'Rate']);
			expect(result.fieldMap['Name']).toBe('name');
			expect(result.fieldMap['SKU']).toBe('sku');
			expect(result.fieldMap['Rate']).toBe('rate');
		});

		it('returns empty arrays for tiers/metadata when all mapped by fuzzy', () => {
			const result = autoDetectMapping(['name', 'sku', 'rate']);
			expect(result.suggestedNewTiers).toEqual([]);
			expect(result.suggestedMetadata).toEqual([]);
		});
	});

	describe('smart data-driven detection', () => {
		it('assigns rate when column has currency values', () => {
			const headers = ['Product Name', 'Cost'];
			const sampleRows = [
				{ 'Product Name': 'Widget A', 'Cost': '$10.50' },
				{ 'Product Name': 'Widget B', 'Cost': '$25.00' },
				{ 'Product Name': 'Widget C', 'Cost': '$5.99' }
			];
			const result = autoDetectMapping(headers, sampleRows);
			expect(result.fieldMap['Cost']).toBe('rate');
		});

		it('assigns name to descriptive text column', () => {
			const headers = ['Code', 'Long Description'];
			const sampleRows = [
				{ 'Code': 'ABC-001', 'Long Description': 'This is a very long descriptive service item name' },
				{ 'Code': 'ABC-002', 'Long Description': 'Another service with a long descriptive name here' }
			];
			const result = autoDetectMapping(headers, sampleRows);
			expect(result.fieldMap['Long Description']).toBe('name');
		});

		it('handles empty sample rows', () => {
			const result = autoDetectMapping(['Col1', 'Col2'], []);
			expect(result.suggestedNewTiers).toEqual([]);
			expect(result.suggestedMetadata).toEqual([]);
		});

		it('does not throw for unknown columns without sample rows', () => {
			expect(() => autoDetectMapping(['UnknownCol1', 'UnknownCol2'])).not.toThrow();
		});
	});
});

describe('applyMapping', () => {
	const basicConfig: ColumnMappingConfig = {
		fieldMap: {
			'Item Name': 'name', 'SKU Code': 'sku', 'Unit': 'unit',
			'Category': 'category', 'Price': 'rate'
		},
		tierColumns: {}, newTierColumns: [], metadataColumns: []
	};

	it('maps fields correctly from a row', () => {
		const rows = [{ 'Item Name': 'Widget', 'SKU Code': 'WID-001', 'Unit': 'ea', 'Category': 'Products', 'Price': '25.00' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?.name).toBe('Widget');
		expect(result[0]?.sku).toBe('WID-001');
		expect(result[0]?.unit).toBe('ea');
		expect(result[0]?.category).toBe('Products');
		expect(result[0]?.rate).toBe(25);
	});

	it('trims whitespace from string fields', () => {
		const rows = [{ 'Item Name': '  Widget  ', 'SKU Code': '  WID-001  ', 'Unit': '  ea  ', 'Category': '  Products  ', 'Price': '25' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?.name).toBe('Widget');
		expect(result[0]?.sku).toBe('WID-001');
		expect(result[0]?.unit).toBe('ea');
		expect(result[0]?.category).toBe('Products');
	});

	it('parses rate with dollar sign', () => {
		const rows = [{ 'Item Name': 'X', 'Price': '$99.99', 'SKU Code': '', 'Unit': '', 'Category': '' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?.rate).toBe(99.99);
	});

	it('parses rate with commas', () => {
		const rows = [{ 'Item Name': 'X', 'Price': '1,500.00', 'SKU Code': '', 'Unit': '', 'Category': '' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?.rate).toBe(1500);
	});

	it('adds error when rate is invalid non-empty string', () => {
		const rows = [{ 'Item Name': 'X', 'Price': 'not-a-rate', 'SKU Code': '', 'Unit': '', 'Category': '' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?._errors).toContainEqual(expect.stringContaining('Invalid rate value'));
	});

	it('adds error when name is missing', () => {
		const rows = [{ 'Item Name': '', 'SKU Code': '', 'Unit': '', 'Category': '', 'Price': '10' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?._errors).toContainEqual('Name is required');
	});

	it('handles skip field without adding errors', () => {
		const config: ColumnMappingConfig = {
			fieldMap: { 'Item Name': 'name', 'Ignore': 'skip' },
			tierColumns: {}, newTierColumns: [], metadataColumns: []
		};
		const rows = [{ 'Item Name': 'Widget', 'Ignore': 'whatever' }];
		const result = applyMapping(rows, config);
		expect(result[0]?._errors).toHaveLength(0);
		expect(result[0]?.name).toBe('Widget');
	});

	it('includes tier rates when tierColumns is configured', () => {
		const config: ColumnMappingConfig = {
			fieldMap: { 'Item Name': 'name' },
			tierColumns: { 'Tier1Price': 1, 'Tier2Price': 2 },
			newTierColumns: [], metadataColumns: []
		};
		const rows = [{ 'Item Name': 'Widget', 'Tier1Price': '50', 'Tier2Price': '40' }];
		const result = applyMapping(rows, config);
		expect(result[0]?.tierRates[1]).toBe(50);
		expect(result[0]?.tierRates[2]).toBe(40);
	});

	it('adds tier error when tier rate is invalid', () => {
		const config: ColumnMappingConfig = {
			fieldMap: { 'Item Name': 'name' },
			tierColumns: { 'Tier1Price': 1 },
			newTierColumns: [], metadataColumns: []
		};
		const rows = [{ 'Item Name': 'Widget', 'Tier1Price': 'bad-value' }];
		const result = applyMapping(rows, config);
		expect(result[0]?._errors).toContainEqual(expect.stringContaining('Invalid tier rate'));
	});

	it('collects metadata columns', () => {
		const config: ColumnMappingConfig = {
			fieldMap: { 'Item Name': 'name' },
			tierColumns: {}, newTierColumns: [], metadataColumns: ['Notes', 'Tags']
		};
		const rows = [{ 'Item Name': 'Widget', 'Notes': 'some notes', 'Tags': '  tag1  ' }];
		const result = applyMapping(rows, config);
		expect(result[0]?.metadata['Notes']).toBe('some notes');
		expect(result[0]?.metadata['Tags']).toBe('tag1');
	});

	it('does not include empty metadata values', () => {
		const config: ColumnMappingConfig = {
			fieldMap: { 'Item Name': 'name' },
			tierColumns: {}, newTierColumns: [], metadataColumns: ['Notes']
		};
		const rows = [{ 'Item Name': 'Widget', 'Notes': '' }];
		const result = applyMapping(rows, config);
		expect(result[0]?.metadata['Notes']).toBeUndefined();
	});

	it('stores raw row in _raw', () => {
		const rows = [{ 'Item Name': 'Widget', 'Price': '10', 'SKU Code': '', 'Unit': '', 'Category': '' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?._raw).toEqual(rows[0]);
	});

	it('handles multiple rows', () => {
		const rows = [
			{ 'Item Name': 'Widget A', 'Price': '10', 'SKU Code': 'A', 'Unit': 'ea', 'Category': 'Cat A' },
			{ 'Item Name': 'Widget B', 'Price': '20', 'SKU Code': 'B', 'Unit': 'hr', 'Category': 'Cat B' }
		];
		const result = applyMapping(rows, basicConfig);
		expect(result).toHaveLength(2);
		expect(result[0]?.name).toBe('Widget A');
		expect(result[1]?.name).toBe('Widget B');
	});

	it('handles empty rows array', () => {
		const result = applyMapping([], basicConfig);
		expect(result).toEqual([]);
	});

	it('defaults rate to 0 when price is empty', () => {
		const rows = [{ 'Item Name': 'Widget', 'Price': '', 'SKU Code': '', 'Unit': '', 'Category': '' }];
		const result = applyMapping(rows, basicConfig);
		expect(result[0]?.rate).toBe(0);
		expect(result[0]?._errors).not.toContainEqual(expect.stringContaining('Invalid rate value'));
	});
});
