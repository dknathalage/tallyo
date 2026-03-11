import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined),
	runRaw: vi.fn()
}));

vi.mock('../audit.js', () => ({
	logAudit: vi.fn(),
	computeChanges: vi.fn().mockReturnValue({})
}));

import { getClients, getClient, createClient, updateClient, deleteClient } from './clients.js';
import { query, execute, save, runRaw } from '../connection.svelte.js';
import { logAudit } from '../audit.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);
const mockRunRaw = vi.mocked(runRaw);
const mockLogAudit = vi.mocked(logAudit);

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

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT c.*, rt.name as pricing_tier_name, p.name as payer_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id LEFT JOIN payers p ON c.payer_id = p.id ORDER BY c.name'
		);
		expect(result).toEqual(clients);
	});

	it('filters clients by search term', () => {
		mockQuery.mockReturnValue([]);

		getClients('alice');

		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT c.*, rt.name as pricing_tier_name, p.name as payer_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id LEFT JOIN payers p ON c.payer_id = p.id WHERE c.name LIKE ? OR c.email LIKE ? ORDER BY c.name',
			['%alice%', '%alice%']
		);
	});
});

describe('getClient', () => {
	it('returns client when found', () => {
		const client = { id: 1, name: 'Alice', email: '', phone: '', address: '', created_at: '', updated_at: '' };
		mockQuery.mockReturnValue([client]);

		expect(getClient(1)).toEqual(client);
		expect(mockQuery).toHaveBeenCalledWith(
			'SELECT c.*, rt.name as pricing_tier_name, p.name as payer_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id LEFT JOIN payers p ON c.payer_id = p.id WHERE c.id = ?',
			[1]
		);
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
			'INSERT INTO clients (uuid, name, email, phone, address, pricing_tier_id, metadata, payer_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)',
			[expect.any(String), 'Bob', 'bob@test.com', '555-0100', '123 Main St', null, '{}', null]
		);
		expect(mockSave).toHaveBeenCalled();
		expect(id).toBe(42);
	});

	it('defaults optional fields to empty string', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createClient({ name: 'Bob' });

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO clients (uuid, name, email, phone, address, pricing_tier_id, metadata, payer_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)',
			[expect.any(String), 'Bob', '', '', '', null, '{}', null]
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

	it('passes metadata and payer_id when provided', async () => {
		mockQuery.mockReturnValue([{ id: 1 }]);

		await createClient({ name: 'Bob', metadata: '{"ABN":"123"}', payer_id: 5 });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO clients'),
			expect.arrayContaining(['{"ABN":"123"}', 5])
		);
	});
});

describe('updateClient', () => {
	it('updates client and saves', async () => {
		await updateClient(1, { name: 'Alice Updated', email: 'alice@new.com' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE clients SET'),
			['Alice Updated', 'alice@new.com', '', '', null, '{}', null, 1]
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
	it('deletes client and audit logs inside a transaction', async () => {
		await deleteClient(5);

		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockExecute).toHaveBeenCalledWith('DELETE FROM clients WHERE id = ?', [5]);
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(mockSave).toHaveBeenCalled();
	});

	it('rolls back when the delete fails', async () => {
		mockExecute.mockImplementationOnce(() => {
			throw new Error('DELETE failed');
		});

		await expect(deleteClient(5)).rejects.toThrow('DELETE failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
	});

	it('rolls back when the audit log write fails', async () => {
		mockLogAudit.mockImplementationOnce(() => { throw new Error('audit write failed'); });

		await expect(deleteClient(5)).rejects.toThrow('audit write failed');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockSave).not.toHaveBeenCalled();
	});
});
