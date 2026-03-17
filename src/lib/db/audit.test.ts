import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn()
}));

import { logAudit, computeChanges } from './audit.js';
import { execute } from './connection.js';

const mockExecute = vi.mocked(execute);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('logAudit', () => {
	it('inserts audit entry with all fields', () => {
		const changes = { name: { old: 'Alice', new: 'Bob' } };

		logAudit({
			entity_type: 'client',
			entity_id: 1,
			action: 'update',
			changes,
			context: 'manual edit',
			batch_id: 'batch-123'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			'INSERT INTO audit_log (uuid, entity_type, entity_id, action, changes, context, batch_id) VALUES (?, ?, ?, ?, ?, ?, ?)',
			[
				expect.any(String),
				'client',
				1,
				'update',
				JSON.stringify(changes),
				'manual edit',
				'batch-123'
			]
		);
	});

	it('defaults entity_id to null when not provided', () => {
		logAudit({
			entity_type: 'invoice',
			action: 'export'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.any(String),
			[
				expect.any(String),
				'invoice',
				null,
				'export',
				'{}',
				'',
				null
			]
		);
	});

	it('defaults changes to empty object JSON', () => {
		logAudit({
			entity_type: 'client',
			entity_id: 5,
			action: 'create'
		});

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args[4]).toBe('{}');
	});

	it('defaults context to empty string', () => {
		logAudit({
			entity_type: 'client',
			entity_id: 1,
			action: 'delete'
		});

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args[5]).toBe('');
	});

	it('defaults batch_id to null', () => {
		logAudit({
			entity_type: 'payment',
			entity_id: 2,
			action: 'create'
		});

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args[6]).toBeNull();
	});

	it('generates a uuid for each call', () => {
		logAudit({ entity_type: 'client', action: 'create' });
		logAudit({ entity_type: 'client', action: 'create' });

		const uuid1 = (mockExecute.mock.calls[0][1] as unknown[])[0];
		const uuid2 = (mockExecute.mock.calls[1][1] as unknown[])[0];
		expect(uuid1).toEqual(expect.any(String));
		expect(uuid2).toEqual(expect.any(String));
		expect(uuid1).not.toBe(uuid2);
	});
});

describe('computeChanges', () => {
	it('returns empty record when no fields changed', () => {
		const result = computeChanges(
			{ name: 'Alice', email: 'a@b.com' },
			{ name: 'Alice', email: 'a@b.com' },
			['name', 'email']
		);

		expect(result).toEqual({});
	});

	it('detects changed fields', () => {
		const result = computeChanges(
			{ name: 'Alice', email: 'old@b.com' },
			{ name: 'Alice', email: 'new@b.com' },
			['name', 'email']
		);

		expect(result).toEqual({
			email: { old: 'old@b.com', new: 'new@b.com' }
		});
	});

	it('detects all fields changed', () => {
		const result = computeChanges(
			{ name: 'Alice', email: 'old@b.com' },
			{ name: 'Bob', email: 'new@b.com' },
			['name', 'email']
		);

		expect(result).toEqual({
			name: { old: 'Alice', new: 'Bob' },
			email: { old: 'old@b.com', new: 'new@b.com' }
		});
	});

	it('handles missing fields in old object', () => {
		const result = computeChanges(
			{},
			{ name: 'Bob' },
			['name']
		);

		expect(result).toEqual({
			name: { old: undefined, new: 'Bob' }
		});
	});

	it('handles missing fields in new object', () => {
		const result = computeChanges(
			{ name: 'Alice' },
			{},
			['name']
		);

		expect(result).toEqual({
			name: { old: 'Alice', new: undefined }
		});
	});

	it('handles null values', () => {
		const result = computeChanges(
			{ name: null },
			{ name: 'Bob' },
			['name']
		);

		expect(result).toEqual({
			name: { old: null, new: 'Bob' }
		});
	});

	it('treats null to null as no change', () => {
		const result = computeChanges(
			{ name: null },
			{ name: null },
			['name']
		);

		expect(result).toEqual({});
	});

	it('only checks specified fields', () => {
		const result = computeChanges(
			{ name: 'Alice', email: 'old@b.com', phone: '111' },
			{ name: 'Bob', email: 'new@b.com', phone: '222' },
			['name']
		);

		expect(result).toEqual({
			name: { old: 'Alice', new: 'Bob' }
		});
		expect(result).not.toHaveProperty('email');
		expect(result).not.toHaveProperty('phone');
	});

	it('handles empty fields array', () => {
		const result = computeChanges(
			{ name: 'Alice' },
			{ name: 'Bob' },
			[]
		);

		expect(result).toEqual({});
	});

	it('distinguishes 0 from null', () => {
		const result = computeChanges(
			{ amount: 0 },
			{ amount: null },
			['amount']
		);

		expect(result).toEqual({
			amount: { old: 0, new: null }
		});
	});

	it('distinguishes empty string from undefined', () => {
		const result = computeChanges(
			{ note: '' },
			{ note: undefined },
			['note']
		);

		expect(result).toEqual({
			note: { old: '', new: undefined }
		});
	});
});
