import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn()
}));

import {
	getColumnMappings,
	getColumnMapping,
	createColumnMapping,
	updateColumnMapping,
	deleteColumnMapping
} from './column-mappings.js';
import { query, execute } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getColumnMappings', () => {
	it('returns all mappings when no entity type specified', () => {
		const mappings = [{ id: 1, name: 'Default', entity_type: 'catalog' }];
		mockQuery.mockReturnValue(mappings);

		const result = getColumnMappings();

		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM column_mappings ORDER BY name');
		expect(result).toEqual(mappings);
	});

	it('filters by entity type when specified', () => {
		mockQuery.mockReturnValue([]);

		getColumnMappings('invoice');

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT * FROM column_mappings WHERE entity_type = ? ORDER BY name',
			['invoice']
		);
	});

	it('returns empty array when no mappings exist', () => {
		mockQuery.mockReturnValue([]);

		expect(getColumnMappings()).toEqual([]);
	});
});

describe('getColumnMapping', () => {
	it('returns mapping when found', () => {
		const mapping = { id: 1, name: 'Default', entity_type: 'catalog' };
		mockQuery.mockReturnValue([mapping]);

		expect(getColumnMapping(1)).toEqual(mapping);
		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM column_mappings WHERE id = ?', [1]);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		expect(getColumnMapping(999)).toBeNull();
	});
});

describe('createColumnMapping', () => {
	it('inserts mapping with all fields and returns id', () => {
		mockQuery.mockReturnValue([{ id: 5 }]);

		const id = createColumnMapping({
			name: 'My Mapping',
			entity_type: 'invoice',
			mapping: { col1: 'field1' },
			tier_mapping: { tier1: 1 },
			metadata_mapping: ['meta1'],
			file_type: 'xlsx',
			sheet_name: 'Sheet1',
			header_row: 2
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO column_mappings'),
			[
				expect.any(String),
				'My Mapping',
				'invoice',
				'{"col1":"field1"}',
				'{"tier1":1}',
				'["meta1"]',
				'xlsx',
				'Sheet1',
				2
			]
		);
		expect(id).toBe(5);
	});

	it('uses default values for optional fields', () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		createColumnMapping({ name: 'Basic', mapping: { a: 'b' } });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO column_mappings'),
			[
				expect.any(String),
				'Basic',
				'catalog',
				'{"a":"b"}',
				'{}',
				'[]',
				'csv',
				'',
				1
			]
		);
	});
});

describe('updateColumnMapping', () => {
	it('updates mapping with all fields', () => {
		updateColumnMapping(3, {
			name: 'Updated',
			entity_type: 'client',
			mapping: { x: 'y' },
			tier_mapping: { t: 2 },
			metadata_mapping: ['m1'],
			file_type: 'csv',
			sheet_name: 'Data',
			header_row: 3
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE column_mappings SET'),
			['Updated', 'client', '{"x":"y"}', '{"t":2}', '["m1"]', 'csv', 'Data', 3, 3]
		);
	});

	it('uses default values for optional fields', () => {
		updateColumnMapping(1, { name: 'Simple', mapping: { a: 'b' } });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE column_mappings SET'),
			['Simple', 'catalog', '{"a":"b"}', '{}', '[]', 'csv', '', 1, 1]
		);
	});
});

describe('deleteColumnMapping', () => {
	it('deletes mapping by id', () => {
		deleteColumnMapping(7);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM column_mappings WHERE id = ?', [7]);
	});
});
