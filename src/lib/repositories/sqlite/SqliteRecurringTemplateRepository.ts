import {
	getRecurringTemplates,
	getRecurringTemplate,
	getDueTemplates,
	createRecurringTemplate,
	updateRecurringTemplate,
	deleteRecurringTemplate,
	advanceTemplateNextDue,
	createInvoiceFromTemplate
} from '$lib/db/queries/recurring-templates.js';
import type { RecurringTemplateRepository, CreateRecurringTemplateInput, UpdateRecurringTemplateInput } from '../interfaces/RecurringTemplateRepository.js';
import type { RecurringTemplate } from '$lib/types/index.js';

export class SqliteRecurringTemplateRepository implements RecurringTemplateRepository {
	getRecurringTemplates(activeOnly = true): RecurringTemplate[] {
		return getRecurringTemplates(activeOnly);
	}

	getRecurringTemplate(id: number): RecurringTemplate | null {
		return getRecurringTemplate(id);
	}

	getDueTemplates(): RecurringTemplate[] {
		return getDueTemplates();
	}

	async createRecurringTemplate(data: CreateRecurringTemplateInput): Promise<number> {
		return createRecurringTemplate(data);
	}

	async updateRecurringTemplate(id: number, data: UpdateRecurringTemplateInput): Promise<void> {
		return updateRecurringTemplate(id, data);
	}

	async deleteRecurringTemplate(id: number): Promise<void> {
		return deleteRecurringTemplate(id);
	}

	async advanceTemplateNextDue(id: number): Promise<void> {
		return advanceTemplateNextDue(id);
	}

	async createInvoiceFromTemplate(templateId: number): Promise<number> {
		return createInvoiceFromTemplate(templateId);
	}
}
