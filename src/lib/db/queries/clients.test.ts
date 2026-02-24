import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

import { getClients, getClient, createClient, updateClient, deleteClient } from './clients.js';
import { query, execute, save } from '../connection.svelte.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getClients', () => {
	it('returns all clients ordered by name when no search', () => {
		const clients = [
			{ id: 1, name: 'Alice', email: '', phone: '', address: '', created_at: '', updated_at: '' }
		];
		mockQuery.mockReturnValue(clients);

		const result = getClients();

		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM clients ORDER BY name');
		expect(result).toEqual(clients);
	});

	it('filters clients by search term', () => {
		mockQuery.mockReturnValue([]);

		getClients('alice');

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT * FROM clients WHERE name LIKE ? OR email LIKE ? ORDER BY name',
			['%alice%', '%alice%']
		);
	});
});

describe('getClient', () => {
	it('returns client when found', () => {
		const client = { id: 1, name: 'Alice', email: '', phone: '', address: '', created_at: '', updated_at: '' };
		mockQuery.mockReturnValue([client]);

		expect(getClient(1)).toEqual(client);
		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM clients WHERE id = ?', [1]);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		expect(getClient(999)).toBeNull();
	});
});

describe('createClient', () => {
	it('inserts client and returns id', async () => {
		mockQuery.mockReturnValue([{ id: 42 }]);

		const id = await createClient({ name: 'Bob', email: 'bob@test.com', phone: '555-0100', address: '123 Main St' });

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO clients (name, email, phone, address) VALUES (?, ?, ?, ?)',
			['Bob', 'bob@test.com', '555-0100', '123 Main St']
		);
		expect(mockSave).toHaveBeenCalled();
		expect(id).toBe(42);
	});

	it('defaults optional fields to empty string', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createClient({ name: 'Bob' });

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO clients (name, email, phone, address) VALUES (?, ?, ?, ?)',
			['Bob', '', '', '']
		);
	});

	it('throws when name is empty string', async () => {
		await expect(createClient({ name: '' })).rejects.toThrow('Client name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is only whitespace', async () => {
		await expect(createClient({ name: '   ' })).rejects.toThrow('Client name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is undefined', async () => {
		await expect(createClient({ name: undefined as any })).rejects.toThrow('Client name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});
});

describe('updateClient', () => {
	it('updates client and saves', async () => {
		await updateClient(1, { name: 'Alice Updated', email: 'alice@new.com' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE clients SET'),
			['Alice Updated', 'alice@new.com', '', '', 1]
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('throws when name is empty string', async () => {
		await expect(updateClient(1, { name: '' })).rejects.toThrow('Client name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});

	it('throws when name is only whitespace', async () => {
		await expect(updateClient(1, { name: '  ' })).rejects.toThrow('Client name is required');
		expect(mockExecute).not.toHaveBeenCalled();
	});
});

describe('deleteClient', () => {
	it('deletes client and saves', async () => {
		await deleteClient(5);

		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM clients WHERE id = ?', [5]);
		expect(mockSave).toHaveBeenCalled();
	});
});
