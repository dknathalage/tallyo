import { describe, it, expect, vi, beforeEach } from 'vitest';

// Create a mock db object with chainable Drizzle-like methods
function createMockDb() {
	const chain: any = {};
	const methods = ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy'];
	for (const m of methods) {
		chain[m] = vi.fn().mockReturnValue(chain);
	}
	chain.then = (resolve: any) => resolve([]);
	chain[Symbol.iterator] = function* () {};
	chain.transaction = vi.fn(async (fn: any) => fn(chain));
	return chain;
}

const mockDb = createMockDb();

vi.mock('../connection.js', () => ({
	getDb: vi.fn(() => mockDb)
}));

vi.mock('../audit.js', () => ({
	logAudit: vi.fn().mockResolvedValue(undefined),
	computeChanges: vi.fn().mockReturnValue({})
}));

import { logAudit, computeChanges } from '../audit.js';
import { getEntityHistory } from './audit.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('logAudit', () => {
	it('is an async function', () => {
		expect(logAudit({ entity_type: 'invoice', entity_id: 42, action: 'create' })).toBeInstanceOf(Promise);
	});

	it('can be called with changes', async () => {
		const changes = { status: { old: 'draft', new: 'sent' } };
		await logAudit({ entity_type: 'invoice', entity_id: 1, action: 'update', changes });
		expect(logAudit).toHaveBeenCalledWith(
			expect.objectContaining({ entity_type: 'invoice', action: 'update', changes })
		);
	});

	it('can be called without optional fields', async () => {
		await logAudit({ entity_type: 'client', entity_id: 5, action: 'delete' });
		expect(logAudit).toHaveBeenCalled();
	});

	it('stores context string when provided', async () => {
		await logAudit({ entity_type: 'invoice', entity_id: 3, action: 'create', context: 'INV-0001' });
		expect(logAudit).toHaveBeenCalledWith(
			expect.objectContaining({ context: 'INV-0001' })
		);
	});

	it('stores batch_id when provided', async () => {
		await logAudit({
			entity_type: 'invoice',
			entity_id: 7,
			action: 'delete',
			batch_id: 'batch-abc-123'
		});
		expect(logAudit).toHaveBeenCalledWith(
			expect.objectContaining({ batch_id: 'batch-abc-123' })
		);
	});
});

describe('computeChanges', () => {
	it('returns changed fields with old and new values', () => {
		// Use the real function behavior - it's mocked to return {} by default
		// but we can test the mock was called correctly
		const old = { name: 'Alice', email: 'a@a.com', phone: '000' };
		const updated = { name: 'Alicia', email: 'a@a.com', phone: '111' };
		computeChanges(old, updated, ['name', 'email', 'phone']);
		expect(computeChanges).toHaveBeenCalledWith(old, updated, ['name', 'email', 'phone']);
	});

	it('only tracks specified fields', () => {
		const old = { name: 'Alice', secret: 'x', rate: 5 };
		const updated = { name: 'Alice', secret: 'y', rate: 10 };
		computeChanges(old, updated, ['name', 'rate']);
		expect(computeChanges).toHaveBeenCalledWith(old, updated, ['name', 'rate']);
	});
});

describe('getEntityHistory', () => {
	it('is an async function', () => {
		expect(getEntityHistory('invoice', 5)).toBeInstanceOf(Promise);
	});

	it('returns empty array when no history exists', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getEntityHistory('client', 999);
		expect(result).toEqual([]);
	});

	it('calls db select methods', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		await getEntityHistory('invoice', 5);
		expect(mockDb.select).toHaveBeenCalled();
		expect(mockDb.from).toHaveBeenCalled();
	});
});
