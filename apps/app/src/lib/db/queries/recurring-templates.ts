import { getDb } from '../connection.js';
import { recurringTemplates, invoices, lineItems, clients } from '../drizzle-schema.js';
import { eq, and, sql, desc } from 'drizzle-orm';
import { logAudit } from '../audit.js';
import { generateInvoiceNumber } from '../number-generators.js';
import { getClient } from './clients.js';
import { getBusinessProfile } from './business-profile.js';
import type { RecurringTemplate, RecurringFrequency } from '../../types/index.js';

function toISOString(d: Date | string | null | undefined): string {
	if (!d) return '';
	if (d instanceof Date) return d.toISOString();
	return d;
}

function mapRowToTemplate(row: Record<string, unknown>): RecurringTemplate {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		client_id: row.client_id as number,
		client_name: (row.client_name as string) ?? undefined,
		name: row.name as string,
		frequency: row.frequency as RecurringFrequency,
		next_due: row.next_due as string,
		line_items: row.line_items as string,
		tax_rate: row.tax_rate as number,
		notes: (row.notes as string) ?? '',
		is_active: (row.is_active as boolean) ? 1 : 0,
		created_at: toISOString(row.created_at as Date | string | null),
		updated_at: toISOString(row.updated_at as Date | string | null)
	};
}

const templateSelectFields = {
	id: recurringTemplates.id,
	uuid: recurringTemplates.uuid,
	client_id: recurringTemplates.client_id,
	client_name: clients.name,
	name: recurringTemplates.name,
	frequency: recurringTemplates.frequency,
	next_due: recurringTemplates.next_due,
	line_items: recurringTemplates.line_items,
	tax_rate: recurringTemplates.tax_rate,
	notes: recurringTemplates.notes,
	is_active: recurringTemplates.is_active,
	created_at: recurringTemplates.created_at,
	updated_at: recurringTemplates.updated_at
};

export async function getRecurringTemplates(activeOnly = true): Promise<RecurringTemplate[]> {
	const db = getDb();

	if (activeOnly) {
		const rows = await db
			.select(templateSelectFields)
			.from(recurringTemplates)
			.leftJoin(clients, eq(recurringTemplates.client_id, clients.id))
			.where(eq(recurringTemplates.is_active, true))
			.orderBy(recurringTemplates.next_due);
		return rows.map((r) => mapRowToTemplate(r as unknown as Record<string, unknown>));
	}

	const rows = await db
		.select(templateSelectFields)
		.from(recurringTemplates)
		.leftJoin(clients, eq(recurringTemplates.client_id, clients.id))
		.orderBy(desc(recurringTemplates.is_active), recurringTemplates.next_due);
	return rows.map((r) => mapRowToTemplate(r as unknown as Record<string, unknown>));
}

export async function getRecurringTemplate(id: number): Promise<RecurringTemplate | null> {
	const db = getDb();
	const rows = await db
		.select(templateSelectFields)
		.from(recurringTemplates)
		.leftJoin(clients, eq(recurringTemplates.client_id, clients.id))
		.where(eq(recurringTemplates.id, id));

	if (rows.length === 0) return null;
	return mapRowToTemplate(rows[0] as unknown as Record<string, unknown>);
}

export async function getDueTemplates(): Promise<RecurringTemplate[]> {
	const db = getDb();
	const today = new Date().toISOString().slice(0, 10);

	const rows = await db
		.select(templateSelectFields)
		.from(recurringTemplates)
		.leftJoin(clients, eq(recurringTemplates.client_id, clients.id))
		.where(
			and(
				eq(recurringTemplates.is_active, true),
				sql`${recurringTemplates.next_due} <= ${today}`
			)
		)
		.orderBy(recurringTemplates.next_due);

	return rows.map((r) => mapRowToTemplate(r as unknown as Record<string, unknown>));
}

export async function createRecurringTemplate(data: {
	client_id: number;
	name: string;
	frequency: RecurringFrequency;
	next_due: string;
	line_items: string;
	tax_rate?: number;
	notes?: string;
	is_active?: number;
}): Promise<number> {
	const db = getDb();

	const [inserted] = await db
		.insert(recurringTemplates)
		.values({
			uuid: crypto.randomUUID(),
			client_id: data.client_id,
			name: data.name,
			frequency: data.frequency,
			next_due: data.next_due,
			line_items: data.line_items,
			tax_rate: data.tax_rate ?? 0,
			notes: data.notes ?? '',
			is_active: (data.is_active ?? 1) === 1
		})
		.returning({ id: recurringTemplates.id });

	await logAudit({
		entity_type: 'recurring_template',
		entity_id: inserted.id,
		action: 'create',
		changes: { name: { old: null, new: data.name } }
	});

	return inserted.id;
}

export async function updateRecurringTemplate(
	id: number,
	data: {
		client_id: number;
		name: string;
		frequency: RecurringFrequency;
		next_due: string;
		line_items: string;
		tax_rate?: number;
		notes?: string;
		is_active?: number;
	}
): Promise<void> {
	const db = getDb();

	await db
		.update(recurringTemplates)
		.set({
			client_id: data.client_id,
			name: data.name,
			frequency: data.frequency,
			next_due: data.next_due,
			line_items: data.line_items,
			tax_rate: data.tax_rate ?? 0,
			notes: data.notes ?? '',
			is_active: (data.is_active ?? 1) === 1,
			updated_at: new Date()
		})
		.where(eq(recurringTemplates.id, id));

	await logAudit({
		entity_type: 'recurring_template',
		entity_id: id,
		action: 'update',
		changes: { name: { old: null, new: data.name } }
	});
}

export async function deleteRecurringTemplate(id: number): Promise<void> {
	const db = getDb();
	await db.delete(recurringTemplates).where(eq(recurringTemplates.id, id));

	await logAudit({
		entity_type: 'recurring_template',
		entity_id: id,
		action: 'delete',
		changes: {}
	});
}

/** Advance next_due by frequency period */
export function advanceNextDue(date: string, frequency: RecurringFrequency): string {
	const d = new Date(date);
	switch (frequency) {
		case 'weekly':
			d.setDate(d.getDate() + 7);
			break;
		case 'monthly':
			d.setMonth(d.getMonth() + 1);
			break;
		case 'quarterly':
			d.setMonth(d.getMonth() + 3);
			break;
	}
	return d.toISOString().slice(0, 10);
}

export async function advanceTemplateNextDue(id: number): Promise<void> {
	const template = await getRecurringTemplate(id);
	if (!template) return;
	const newDate = advanceNextDue(template.next_due, template.frequency);
	const db = getDb();
	await db
		.update(recurringTemplates)
		.set({ next_due: newDate, updated_at: new Date() })
		.where(eq(recurringTemplates.id, id));
}

/** Create a draft invoice from a recurring template and advance next_due */
export async function createInvoiceFromTemplate(templateId: number): Promise<number> {
	const template = await getRecurringTemplate(templateId);
	if (!template) throw new Error(`Recurring template ${templateId} not found`);

	const invoiceNumber = await generateInvoiceNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	// Build snapshots
	const profile = await getBusinessProfile();
	const businessSnap = JSON.stringify({
		name: profile?.name ?? '',
		email: profile?.email ?? '',
		phone: profile?.phone ?? '',
		address: profile?.address ?? '',
		logo: profile?.logo ?? '',
		metadata: {}
	});

	const client = template.client_id ? await getClient(template.client_id) : null;
	const clientSnap = JSON.stringify({
		name: client?.name ?? '',
		email: client?.email ?? '',
		phone: client?.phone ?? '',
		address: client?.address ?? '',
		metadata: {}
	});

	// Parse line items from template JSON
	interface TemplateLineItem {
		description: string;
		quantity: number;
		rate: number;
		amount: number;
		notes?: string;
		sort_order?: number;
	}
	let templateLineItems: TemplateLineItem[] = [];
	try {
		templateLineItems = JSON.parse(template.line_items);
	} catch {
		templateLineItems = [];
	}

	// Calculate totals from line items
	const subtotal = templateLineItems.reduce(
		(sum: number, li: TemplateLineItem) => sum + (li.amount ?? li.quantity * li.rate),
		0
	);
	const taxAmount = subtotal * (template.tax_rate / 100);
	const total = subtotal + taxAmount;

	const defaultCurrency = profile?.default_currency ?? 'USD';

	const db = getDb();
	const invoiceId = await db.transaction(async (tx) => {
		const [inserted] = await tx
			.insert(invoices)
			.values({
				uuid: crypto.randomUUID(),
				invoice_number: invoiceNumber,
				client_id: template.client_id,
				date: todayStr,
				due_date: todayStr,
				payment_terms: 'custom',
				subtotal,
				tax_rate: template.tax_rate,
				tax_amount: taxAmount,
				total,
				notes: template.notes ?? '',
				status: 'draft',
				currency_code: defaultCurrency,
				business_snapshot: businessSnap,
				client_snapshot: clientSnap,
				payer_snapshot: '{}'
			})
			.returning({ id: invoices.id });

		const newInvoiceId = inserted.id;

		for (let i = 0; i < templateLineItems.length; i++) {
			const li = templateLineItems[i];
			await tx.insert(lineItems).values({
				uuid: crypto.randomUUID(),
				invoice_id: newInvoiceId,
				description: li.description,
				quantity: li.quantity,
				rate: li.rate,
				amount: li.amount ?? li.quantity * li.rate,
				notes: li.notes ?? '',
				sort_order: li.sort_order ?? i
			});
		}

		return newInvoiceId;
	});

	await logAudit({
		entity_type: 'invoice',
		entity_id: invoiceId,
		action: 'create',
		context: `${invoiceNumber} (from recurring template: ${template.name})`
	});

	// Advance template next_due
	const newDate = advanceNextDue(template.next_due, template.frequency);
	await db
		.update(recurringTemplates)
		.set({ next_due: newDate, updated_at: new Date() })
		.where(eq(recurringTemplates.id, templateId));

	return invoiceId;
}
