import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/recurring-templates.js', () => ({
	getRecurringTemplates: vi.fn(),
	getRecurringTemplate: vi.fn(),
	getDueTemplates: vi.fn(),
	createRecurringTemplate: vi.fn(),
	updateRecurringTemplate: vi.fn(),
	deleteRecurringTemplate: vi.fn(),
	advanceTemplateNextDue: vi.fn(),
	createInvoiceFromTemplate: vi.fn()
}));

import { SqliteRecurringTemplateRepository } from './SqliteRecurringTemplateRepository.js';
import * as queries from '$lib/db/queries/recurring-templates.js';

const mockGetRecurringTemplates = vi.mocked(queries.getRecurringTemplates);
const mockGetRecurringTemplate = vi.mocked(queries.getRecurringTemplate);
const mockGetDueTemplates = vi.mocked(queries.getDueTemplates);
const mockCreateRecurringTemplate = vi.mocked(queries.createRecurringTemplate);
const mockUpdateRecurringTemplate = vi.mocked(queries.updateRecurringTemplate);
const mockDeleteRecurringTemplate = vi.mocked(queries.deleteRecurringTemplate);
const mockAdvanceTemplateNextDue = vi.mocked(queries.advanceTemplateNextDue);
const mockCreateInvoiceFromTemplate = vi.mocked(queries.createInvoiceFromTemplate);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteRecurringTemplateRepository', () => {
	describe('getRecurringTemplates', () => {
		it('delegates with activeOnly=true by default', () => {
			const repo = new SqliteRecurringTemplateRepository();
			const templates = [{ id: 1, name: 'Monthly' }] as any;
			mockGetRecurringTemplates.mockReturnValue(templates);

			const result = repo.getRecurringTemplates();
			expect(mockGetRecurringTemplates).toHaveBeenCalledWith(true);
			expect(result).toBe(templates);
		});

		it('passes activeOnly=false', () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockGetRecurringTemplates.mockReturnValue([]);

			repo.getRecurringTemplates(false);
			expect(mockGetRecurringTemplates).toHaveBeenCalledWith(false);
		});
	});

	describe('getRecurringTemplate', () => {
		it('delegates to getRecurringTemplate query', () => {
			const repo = new SqliteRecurringTemplateRepository();
			const template = { id: 1, name: 'Monthly' } as any;
			mockGetRecurringTemplate.mockReturnValue(template);

			expect(repo.getRecurringTemplate(1)).toBe(template);
			expect(mockGetRecurringTemplate).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockGetRecurringTemplate.mockReturnValue(null);
			expect(repo.getRecurringTemplate(999)).toBeNull();
		});
	});

	describe('getDueTemplates', () => {
		it('delegates to getDueTemplates query', () => {
			const repo = new SqliteRecurringTemplateRepository();
			const templates = [{ id: 1, name: 'Monthly' }] as any;
			mockGetDueTemplates.mockReturnValue(templates);

			const result = repo.getDueTemplates();
			expect(mockGetDueTemplates).toHaveBeenCalled();
			expect(result).toBe(templates);
		});
	});

	describe('createRecurringTemplate', () => {
		it('delegates to createRecurringTemplate query', async () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockCreateRecurringTemplate.mockResolvedValue(4);

			const data = { name: 'Monthly', client_id: 1, frequency: 'monthly' } as any;
			const id = await repo.createRecurringTemplate(data);

			expect(mockCreateRecurringTemplate).toHaveBeenCalledWith(data);
			expect(id).toBe(4);
		});
	});

	describe('updateRecurringTemplate', () => {
		it('delegates to updateRecurringTemplate query', async () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockUpdateRecurringTemplate.mockResolvedValue(undefined);

			const data = { name: 'Monthly Updated' } as any;
			await repo.updateRecurringTemplate(1, data);

			expect(mockUpdateRecurringTemplate).toHaveBeenCalledWith(1, data);
		});
	});

	describe('deleteRecurringTemplate', () => {
		it('delegates to deleteRecurringTemplate query', async () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockDeleteRecurringTemplate.mockResolvedValue(undefined);

			await repo.deleteRecurringTemplate(2);
			expect(mockDeleteRecurringTemplate).toHaveBeenCalledWith(2);
		});
	});

	describe('advanceTemplateNextDue', () => {
		it('delegates to advanceTemplateNextDue query', async () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockAdvanceTemplateNextDue.mockResolvedValue(undefined);

			await repo.advanceTemplateNextDue(1);
			expect(mockAdvanceTemplateNextDue).toHaveBeenCalledWith(1);
		});
	});

	describe('createInvoiceFromTemplate', () => {
		it('delegates to createInvoiceFromTemplate query', async () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockCreateInvoiceFromTemplate.mockResolvedValue(10);

			const id = await repo.createInvoiceFromTemplate(3);
			expect(mockCreateInvoiceFromTemplate).toHaveBeenCalledWith(3);
			expect(id).toBe(10);
		});

		it('propagates errors', async () => {
			const repo = new SqliteRecurringTemplateRepository();
			mockCreateInvoiceFromTemplate.mockRejectedValue(new Error('template inactive'));

			await expect(repo.createInvoiceFromTemplate(99)).rejects.toThrow('template inactive');
		});
	});
});
