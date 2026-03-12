import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn(),
	execute: vi.fn()
}));

import { getPayers, getPayer, createPayer, updatePayer, deletePayer, buildPayerSnapshot } from './payers.js';
import { query, execute } from '../connection.svelte.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getPayers', () => {
	it('returns all payers ordered by name', () => {
		const payers = [{ id: 1, name: 'Payer A' }];
		mockQuery.mockReturnValue(payers);

		expect(getPayers()).toEqual(payers);
		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM payers ORDER BY name');
	});

	it('filters by search term', () => {
		mockQuery.mockReturnValue([]);

		getPayers('test');

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT * FROM payers WHERE name LIKE ? OR email LIKE ? ORDER BY name',
			['%test%', '%test%']
		);
	});
});

describe('getPayer', () => {
	it('returns payer when found', () => {
		const payer = { id: 1, name: 'Payer A' };
		mockQuery.mockReturnValue([payer]);

		expect(getPayer(1)).toEqual(payer);
		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM payers WHERE id = ?', [1]);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		expect(getPayer(999)).toBeNull();
	});
});

describe('createPayer', () => {
	it('inserts payer and returns id', async () => {
		mockQuery.mockReturnValue([{ id: 10 }]);

		const id = await createPayer({ name: 'New Payer', email: 'payer@test.com' });

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO payers (uuid, name, email, phone, address, metadata) VALUES (?, ?, ?, ?, ?, ?)',
			[expect.any(String), 'New Payer', 'payer@test.com', '', '', '{}']
		);
		// save() is now the repository's responsibility, not the query fn's
		expect(id).toBe(10);
	});

	it('throws when name is empty', async () => {
		await expect(createPayer({ name: '' })).rejects.toThrow('Payer name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is whitespace', async () => {
		await expect(createPayer({ name: '   ' })).rejects.toThrow('Payer name is required');
	});
});

describe('updatePayer', () => {
	it('updates payer', async () => {
		await updatePayer(1, { name: 'Updated Payer', email: 'new@test.com' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE payers SET'),
			['Updated Payer', 'new@test.com', '', '', '{}', 1]
		);
		// save() is now the repository's responsibility
	});

	it('throws when name is empty', async () => {
		await expect(updatePayer(1, { name: '' })).rejects.toThrow('Payer name is required');
	});
});

describe('deletePayer', () => {
	it('deletes payer', async () => {
		await deletePayer(5);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM payers WHERE id = ?', [5]);
		// save() and logAudit() are now the repository's responsibility
	});
});

describe('buildPayerSnapshot', () => {
	it('returns empty snapshot for null payerId', () => {
		expect(buildPayerSnapshot(null)).toEqual({
			name: '',
			email: '',
			phone: '',
			address: '',
			metadata: {}
		});
	});

	it('returns empty snapshot when payer not found', () => {
		mockQuery.mockReturnValue([]);

		expect(buildPayerSnapshot(999)).toEqual({
			name: '',
			email: '',
			phone: '',
			address: '',
			metadata: {}
		});
	});

	it('builds snapshot from payer with metadata', () => {
		mockQuery.mockReturnValue([{
			id: 1,
			name: 'NDIA',
			email: 'ndia@gov.au',
			phone: '1800-800-110',
			address: '123 Gov St',
			metadata: '{"Funding Ref":"FR-789"}'
		}]);

		expect(buildPayerSnapshot(1)).toEqual({
			name: 'NDIA',
			email: 'ndia@gov.au',
			phone: '1800-800-110',
			address: '123 Gov St',
			metadata: { 'Funding Ref': 'FR-789' }
		});
	});
});
