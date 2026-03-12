import type { RecurringTemplate, RecurringFrequency } from '$lib/types/index.js';

export interface CreateRecurringTemplateInput {
	client_id: number;
	name: string;
	frequency: RecurringFrequency;
	next_due: string;
	line_items: string;
	tax_rate?: number;
	notes?: string;
	is_active?: number;
}

export type UpdateRecurringTemplateInput = CreateRecurringTemplateInput;

export interface RecurringTemplateRepository {
	getRecurringTemplates(activeOnly?: boolean): RecurringTemplate[];
	getRecurringTemplate(id: number): RecurringTemplate | null;
	getDueTemplates(): RecurringTemplate[];

	createRecurringTemplate(data: CreateRecurringTemplateInput): Promise<number>;
	updateRecurringTemplate(id: number, data: UpdateRecurringTemplateInput): Promise<void>;
	deleteRecurringTemplate(id: number): Promise<void>;
	advanceTemplateNextDue(id: number): Promise<void>;
	createInvoiceFromTemplate(templateId: number): Promise<number>;
}
