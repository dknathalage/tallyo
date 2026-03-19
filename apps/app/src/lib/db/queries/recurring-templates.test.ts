import { describe, it, expect, vi, beforeEach } from 'vitest';

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

import {
	getRecurringTemplates,
	getRecurringTemplate,
	getDueTemplates,
	createRecurringTemplate,
	updateRecurringTemplate,
	deleteRecurringTemplate,
	advanceNextDue
} from './recurring-templates.js';

beforeEach(() => {
	vi.clearAllMocks();
	mockDb.then = (resolve: any) => resolve([]);
	for (const m of ['select', 'insert', 'update', 'delete', 'from', 'where', 'set', 'values',
		'returning', 'orderBy', 'leftJoin', 'limit', 'groupBy']) {
		mockDb[m].mockReturnValue(mockDb);
	}
});

describe('getRecurringTemplates', () => {
	it('is an async function', () => {
		expect(getRecurringTemplates()).toBeInstanceOf(Promise);
	});

	it('returns empty array when no templates', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getRecurringTemplates();
		expect(result).toEqual([]);
	});
});

describe('getRecurringTemplate', () => {
	it('is an async function', () => {
		expect(getRecurringTemplate(1)).toBeInstanceOf(Promise);
	});

	it('returns null when not found', async () => {
		mockDb.then = (resolve: any) => resolve([]);
		const result = await getRecurringTemplate(999);
		expect(result).toBeNull();
	});
});

describe('getDueTemplates', () => {
	it('is an async function', () => {
		expect(getDueTemplates()).toBeInstanceOf(Promise);
	});
});

describe('createRecurringTemplate', () => {
	it('is an async function', () => {
		mockDb.returning.mockResolvedValue([{ id: 5 }]);
		expect(createRecurringTemplate({
			client_id: 1,
			name: 'Monthly Retainer',
			frequency: 'monthly',
			next_due: '2026-04-01',
			line_items: '[]'
		})).toBeInstanceOf(Promise);
	});

	it('returns an id', async () => {
		mockDb.returning.mockResolvedValue([{ id: 5 }]);
		const id = await createRecurringTemplate({
			client_id: 1,
			name: 'Monthly Retainer',
			frequency: 'monthly',
			next_due: '2026-04-01',
			line_items: '[]'
		});
		expect(id).toBe(5);
	});
});

describe('updateRecurringTemplate', () => {
	it('is an async function', () => {
		expect(updateRecurringTemplate(1, {
			client_id: 1,
			name: 'Updated',
			frequency: 'weekly',
			next_due: '2026-03-20',
			line_items: '[]'
		})).toBeInstanceOf(Promise);
	});
});

describe('deleteRecurringTemplate', () => {
	it('is an async function', () => {
		expect(deleteRecurringTemplate(3)).toBeInstanceOf(Promise);
	});
});

describe('advanceNextDue', () => {
	it('advances weekly by 7 days', () => {
		expect(advanceNextDue('2026-03-12', 'weekly')).toBe('2026-03-19');
	});

	it('advances monthly by 1 month', () => {
		expect(advanceNextDue('2026-03-01', 'monthly')).toBe('2026-04-01');
	});

	it('advances quarterly by 3 months', () => {
		expect(advanceNextDue('2026-01-01', 'quarterly')).toBe('2026-04-01');
	});
});
