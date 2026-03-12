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

import {
	getRecurringTemplates,
	getRecurringTemplate,
	getDueTemplates,
	createRecurringTemplate,
	updateRecurringTemplate,
	deleteRecurringTemplate,
	advanceNextDue
} from './recurring-templates.js';
import { query, execute, save } from '../connection.svelte.js';
import { logAudit } from '../audit.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);
const mockLogAudit = vi.mocked(logAudit);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getRecurringTemplates', () => {
	it('returns active templates by default', () => {
		const templates = [{ id: 1, name: 'Monthly', is_active: 1 }];
		mockQuery.mockReturnValue(templates);

		const result = getRecurringTemplates();

		expect(mockQuery).toHaveBeenCalledWith(
			expect.stringContaining('is_active = 1')
		);
		expect(result).toEqual(templates);
	});

	it('returns all templates when activeOnly = false', () => {
		mockQuery.mockReturnValue([]);

		getRecurringTemplates(false);

		const call = mockQuery.mock.calls[0][0] as string;
		expect(call).not.toContain('is_active = 1');
	});
});

describe('getRecurringTemplate', () => {
	it('returns template when found', () => {
		const template = { id: 1, name: 'Monthly' };
		mockQuery.mockReturnValue([template]);

		const result = getRecurringTemplate(1);

		expect(result).toEqual(template);
	});

	it('returns null when not found', () => {
		mockQuery.mockReturnValue([]);

		const result = getRecurringTemplate(999);

		expect(result).toBeNull();
	});
});

describe('getDueTemplates', () => {
	it('queries templates due today or earlier', () => {
		mockQuery.mockReturnValue([]);

		getDueTemplates();

		const call = mockQuery.mock.calls[0][0] as string;
		expect(call).toContain('next_due <=');
		expect(call).toContain('is_active = 1');
	});
});

describe('createRecurringTemplate', () => {
	it('inserts template and returns new id', async () => {
		mockQuery.mockReturnValue([{ id: 5 }]);

		const id = await createRecurringTemplate({
			client_id: 1,
			name: 'Monthly Retainer',
			frequency: 'monthly',
			next_due: '2026-04-01',
			line_items: '[]'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT INTO recurring_templates'),
			expect.arrayContaining(['Monthly Retainer', 'monthly', '2026-04-01'])
		);
		expect(mockSave).toHaveBeenCalled();
		expect(mockLogAudit).toHaveBeenCalledWith(expect.objectContaining({ action: 'create' }));
		expect(id).toBe(5);
	});
});

describe('updateRecurringTemplate', () => {
	it('updates template fields', async () => {
		await updateRecurringTemplate(1, {
			client_id: 1,
			name: 'Updated',
			frequency: 'weekly',
			next_due: '2026-03-20',
			line_items: '[]'
		});

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('UPDATE recurring_templates'),
			expect.arrayContaining(['Updated', 'weekly'])
		);
		expect(mockSave).toHaveBeenCalled();
	});
});

describe('deleteRecurringTemplate', () => {
	it('deletes template by id', async () => {
		await deleteRecurringTemplate(3);

		expect(mockExecute).toHaveBeenCalledWith(
			`DELETE FROM recurring_templates WHERE id = ?`,
			[3]
		);
		expect(mockSave).toHaveBeenCalled();
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
