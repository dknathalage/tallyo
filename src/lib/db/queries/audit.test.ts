import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

import { logAudit, computeChanges } from '../audit.js';
import { getEntityHistory } from './audit.js';
import { query, execute } from '../connection.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('logAudit', () => {
	it('inserts an audit log entry with correct entity_type, action and entity_id', () => {
		logAudit({ entity_type: 'invoice', entity_id: 42, action: 'create' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO audit_log'),
			expect.arrayContaining(['invoice', 42, 'create'])
		);
	});

	it('serialises changes to JSON', () => {
		const changes = { status: { old: 'draft', new: 'sent' } };

		logAudit({ entity_type: 'invoice', entity_id: 1, action: 'update', changes });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args).toContain(JSON.stringify(changes));
	});

	it('stores empty JSON object when no changes provided', () => {
		logAudit({ entity_type: 'client', entity_id: 5, action: 'delete' });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args).toContain('{}');
	});

	it('stores context string when provided', () => {
		logAudit({ entity_type: 'invoice', entity_id: 3, action: 'create', context: 'INV-0001' });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args).toContain('INV-0001');
	});

	it('stores batch_id when provided', () => {
		logAudit({
			entity_type: 'invoice',
			entity_id: 7,
			action: 'delete',
			batch_id: 'batch-abc-123'
		});

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args).toContain('batch-abc-123');
	});

	it('stores null for entity_id when not provided', () => {
		logAudit({ entity_type: 'invoice', action: 'create' });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(args).toContain(null);
	});

	it('inserts a uuid as first parameter', () => {
		logAudit({ entity_type: 'catalog', entity_id: 1, action: 'create' });

		const args = mockExecute.mock.calls[0][1] as unknown[];
		expect(typeof args[0]).toBe('string');
		expect(args[0] as string).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
		);
	});
});

describe('computeChanges', () => {
	it('returns changed fields with old and new values', () => {
		const old = { name: 'Alice', email: 'a@a.com', phone: '000' };
		const updated = { name: 'Alicia', email: 'a@a.com', phone: '111' };

		const changes = computeChanges(old, updated, ['name', 'email', 'phone']);

		expect(changes).toEqual({
			name: { old: 'Alice', new: 'Alicia' },
			phone: { old: '000', new: '111' }
		});
	});

	it('returns empty object when nothing changed', () => {
		const obj = { name: 'Same', rate: 10 };

		const changes = computeChanges(obj, { ...obj }, ['name', 'rate']);

		expect(changes).toEqual({});
	});

	it('only tracks specified fields', () => {
		const old = { name: 'Alice', secret: 'x', rate: 5 };
		const updated = { name: 'Alice', secret: 'y', rate: 10 };

		const changes = computeChanges(old, updated, ['name', 'rate']);

		expect(changes).not.toHaveProperty('secret');
		expect(changes).toHaveProperty('rate');
	});

	it('detects change from a value to null', () => {
		const old = { notes: 'some note' };
		const updated = { notes: null };

		const changes = computeChanges(
			old as Record<string, unknown>,
			updated as Record<string, unknown>,
			['notes']
		);

		expect(changes).toEqual({ notes: { old: 'some note', new: null } });
	});
});

describe('getEntityHistory', () => {
	it('returns audit entries for the specified entity in reverse chronological order', () => {
		const entries = [
			{ id: 2, entity_type: 'invoice', entity_id: 5, action: 'update', created_at: '2025-02-01' },
			{ id: 1, entity_type: 'invoice', entity_id: 5, action: 'create', created_at: '2025-01-01' }
		];
		mockQuery.mockReturnValue(entries);

		const result = getEntityHistory('invoice', 5);

		expect(result).toEqual(entries);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('WHERE entity_type = ? AND entity_id = ?'),
			['invoice', 5]
		);
		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('ORDER BY created_at DESC'),
			['invoice', 5]
		);
	});

	it('returns empty array when no history exists for entity', () => {
		mockQuery.mockReturnValue([]);

		const result = getEntityHistory('client', 999);

		expect(result).toEqual([]);
	});

	it('filters by entity_type correctly', () => {
		mockQuery.mockReturnValue([]);

		getEntityHistory('catalog', 10);

		expect(mockQuery).toHaveBeenCalledWith(expect.any(String), ['catalog', 10]);
	});
});
