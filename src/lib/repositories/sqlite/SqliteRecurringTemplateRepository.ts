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
	async getRecurringTemplates(activeOnly = true): Promise<RecurringTemplate[]> {
		return await getRecurringTemplates(activeOnly);
	}

	async getRecurringTemplate(id: number): Promise<RecurringTemplate | null> {
		return await getRecurringTemplate(id);
	}

	async getDueTemplates(): Promise<RecurringTemplate[]> {
		return await getDueTemplates();
	}

	async createRecurringTemplate(data: CreateRecurringTemplateInput): Promise<number> {
		return await createRecurringTemplate(data);
	}

	async updateRecurringTemplate(id: number, data: UpdateRecurringTemplateInput): Promise<void> {
		return await updateRecurringTemplate(id, data);
	}

	async deleteRecurringTemplate(id: number): Promise<void> {
		return await deleteRecurringTemplate(id);
	}

	async advanceTemplateNextDue(id: number): Promise<void> {
		return await advanceTemplateNextDue(id);
	}

	async createInvoiceFromTemplate(templateId: number): Promise<number> {
		return await createInvoiceFromTemplate(templateId);
	}
}
