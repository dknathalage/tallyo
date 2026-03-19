import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockInsert = vi.fn().mockReturnThis();
const mockValues = vi.fn().mockResolvedValue(undefined);

vi.mock('./connection.js', () => ({
	getDb: vi.fn(() => ({
		insert: mockInsert,
		values: mockValues
	}))
}));

// Make insert().values() chainable
mockInsert.mockReturnValue({ values: mockValues });

import { logAudit, computeChanges } from './audit.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockInsert.mockReturnValue({ values: mockValues });
});

describe('logAudit', () => {
	it('is an async function', () => {
		expect(logAudit({ entity_type: 'client', action: 'create' })).toBeInstanceOf(Promise);
	});

	it('calls db.insert with audit log values', async () => {
		const changes = { name: { old: 'Alice', new: 'Bob' } };
		await logAudit({
			entity_type: 'client',
			entity_id: 1,
			action: 'update',
			changes,
			context: 'manual edit',
			batch_id: 'batch-123'
		});

		expect(mockInsert).toHaveBeenCalled();
		expect(mockValues).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_type: 'client',
				entity_id: 1,
				action: 'update',
				changes: JSON.stringify(changes),
				context: 'manual edit',
				batch_id: 'batch-123'
			})
		);
	});

	it('defaults entity_id to null when not provided', async () => {
		await logAudit({
			entity_type: 'invoice',
			action: 'export'
		});

		expect(mockValues).toHaveBeenCalledWith(
			expect.objectContaining({
				entity_id: null,
				changes: '{}',
				context: '',
				batch_id: null
			})
		);
	});

	it('defaults changes to empty object JSON', async () => {
		await logAudit({
			entity_type: 'client',
			entity_id: 5,
			action: 'create'
		});

		expect(mockValues).toHaveBeenCalledWith(
			expect.objectContaining({ changes: '{}' })
		);
	});

	it('defaults context to empty string', async () => {
		await logAudit({
			entity_type: 'client',
			entity_id: 1,
			action: 'delete'
		});

		expect(mockValues).toHaveBeenCalledWith(
			expect.objectContaining({ context: '' })
		);
	});

	it('defaults batch_id to null', async () => {
		await logAudit({
			entity_type: 'payment',
			entity_id: 2,
			action: 'create'
		});

		expect(mockValues).toHaveBeenCalledWith(
			expect.objectContaining({ batch_id: null })
		);
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
