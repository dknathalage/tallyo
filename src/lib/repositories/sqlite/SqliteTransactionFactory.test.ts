import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/connection.js', () => ({
	runRaw: vi.fn()
}));

import { SqliteTransaction, SqliteTransactionFactory } from './SqliteTransactionFactory.js';
import { runRaw } from '$lib/db/connection.js';

const mockRunRaw = vi.mocked(runRaw);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteTransaction', () => {
	it('begin calls runRaw with BEGIN TRANSACTION', async () => {
		const tx = new SqliteTransaction();
		await tx.begin();
		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
	});

	it('commit calls runRaw with COMMIT', async () => {
		const tx = new SqliteTransaction();
		await tx.commit();
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
	});

	it('rollback calls runRaw with ROLLBACK', async () => {
		const tx = new SqliteTransaction();
		await tx.rollback();
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
	});

	it('run executes fn and commits on success', async () => {
		const tx = new SqliteTransaction();
		const fn = vi.fn().mockResolvedValue(42);

		const result = await tx.run(fn);

		expect(result).toBe(42);
		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockRunRaw).toHaveBeenCalledWith('COMMIT');
		expect(fn).toHaveBeenCalled();
	});

	it('run rolls back and rethrows on error', async () => {
		const tx = new SqliteTransaction();
		const fn = vi.fn().mockRejectedValue(new Error('fn error'));

		await expect(tx.run(fn)).rejects.toThrow('fn error');
		expect(mockRunRaw).toHaveBeenCalledWith('BEGIN TRANSACTION');
		expect(mockRunRaw).toHaveBeenCalledWith('ROLLBACK');
		expect(mockRunRaw).not.toHaveBeenCalledWith('COMMIT');
	});

	it('run does not commit when fn throws', async () => {
		const tx = new SqliteTransaction();
		const fn = vi.fn().mockRejectedValue(new Error('failure'));

		try {
			await tx.run(fn);
		} catch {
			// expected
		}

		const calls = mockRunRaw.mock.calls.map((c) => c[0]);
		expect(calls).not.toContain('COMMIT');
		expect(calls).toContain('ROLLBACK');
	});
});

describe('SqliteTransactionFactory', () => {
	it('create returns a StorageTransaction instance', () => {
		const factory = new SqliteTransactionFactory();
		const tx = factory.create();
		expect(tx).toBeDefined();
		expect(typeof tx.run).toBe('function');
	});

	it('create returns a new instance each time', () => {
		const factory = new SqliteTransactionFactory();
		const tx1 = factory.create();
		const tx2 = factory.create();
		expect(tx1).not.toBe(tx2);
	});
});
