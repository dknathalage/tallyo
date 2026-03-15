import { execute, query, runRaw, save } from '../connection.js';
import { logAudit } from '../audit.js';
import { generateInvoiceNumber } from '../number-generators.js';
import { getClient } from './clients.js';
import { getBusinessProfile } from './business-profile.js';
import type { RecurringTemplate, RecurringFrequency } from '../../types/index.js';

export function getRecurringTemplates(activeOnly = true): RecurringTemplate[] {
	if (activeOnly) {
		return query<RecurringTemplate>(
			`SELECT rt.*, c.name as client_name
			 FROM recurring_templates rt
			 LEFT JOIN clients c ON rt.client_id = c.id
			 WHERE rt.is_active = 1
			 ORDER BY rt.next_due ASC`
		);
	}
	return query<RecurringTemplate>(
		`SELECT rt.*, c.name as client_name
		 FROM recurring_templates rt
		 LEFT JOIN clients c ON rt.client_id = c.id
		 ORDER BY rt.is_active DESC, rt.next_due ASC`
	);
}

export function getRecurringTemplate(id: number): RecurringTemplate | null {
	const results = query<RecurringTemplate>(
		`SELECT rt.*, c.name as client_name
		 FROM recurring_templates rt
		 LEFT JOIN clients c ON rt.client_id = c.id
		 WHERE rt.id = ?`,
		[id]
	);
	return results.length > 0 ? results[0] : null;
}

export function getDueTemplates(): RecurringTemplate[] {
	const today = new Date().toISOString().slice(0, 10);
	return query<RecurringTemplate>(
		`SELECT rt.*, c.name as client_name
		 FROM recurring_templates rt
		 LEFT JOIN clients c ON rt.client_id = c.id
		 WHERE rt.is_active = 1 AND rt.next_due <= ?
		 ORDER BY rt.next_due ASC`,
		[today]
	);
}

export function createRecurringTemplate(data: {
	client_id: number;
	name: string;
	frequency: RecurringFrequency;
	next_due: string;
	line_items: string;
	tax_rate?: number;
	notes?: string;
	is_active?: number;
}): number {
	execute(
		`INSERT INTO recurring_templates (uuid, client_id, name, frequency, next_due, line_items, tax_rate, notes, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			data.client_id,
			data.name,
			data.frequency,
			data.next_due,
			data.line_items,
			data.tax_rate ?? 0,
			data.notes ?? '',
			data.is_active ?? 1
		]
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	logAudit({
		entity_type: 'recurring_template',
		entity_id: result[0].id,
		action: 'create',
		changes: { name: { old: null, new: data.name } }
	});
	save();
	return result[0].id;
}

export function updateRecurringTemplate(
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
): void {
	execute(
		`UPDATE recurring_templates
		 SET client_id = ?, name = ?, frequency = ?, next_due = ?, line_items = ?,
		     tax_rate = ?, notes = ?, is_active = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		[
			data.client_id,
			data.name,
			data.frequency,
			data.next_due,
			data.line_items,
			data.tax_rate ?? 0,
			data.notes ?? '',
			data.is_active ?? 1,
			id
		]
	);
	logAudit({
		entity_type: 'recurring_template',
		entity_id: id,
		action: 'update',
		changes: { name: { old: null, new: data.name } }
	});
	save();
}

export function deleteRecurringTemplate(id: number): void {
	execute(`DELETE FROM recurring_templates WHERE id = ?`, [id]);
	logAudit({ entity_type: 'recurring_template', entity_id: id, action: 'delete', changes: {} });
	save();
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

export function advanceTemplateNextDue(id: number): void {
	const template = getRecurringTemplate(id);
	if (!template) return;
	const newDate = advanceNextDue(template.next_due, template.frequency);
	execute(
		`UPDATE recurring_templates SET next_due = ?, updated_at = datetime('now') WHERE id = ?`,
		[newDate, id]
	);
}

/** Create a draft invoice from a recurring template and advance next_due */
export function createInvoiceFromTemplate(templateId: number): number {
	const template = getRecurringTemplate(templateId);
	if (!template) throw new Error(`Recurring template ${templateId} not found`);

	const invoiceNumber = generateInvoiceNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	// Build snapshots
	const profile = getBusinessProfile();
	const businessSnap = JSON.stringify({
		name: profile?.name ?? '',
		email: profile?.email ?? '',
		phone: profile?.phone ?? '',
		address: profile?.address ?? '',
		logo: profile?.logo ?? '',
		metadata: {}
	});

	const client = template.client_id ? getClient(template.client_id) : null;
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
	let lineItems: TemplateLineItem[] = [];
	try {
		lineItems = JSON.parse(template.line_items);
	} catch {
		lineItems = [];
	}

	// Calculate totals from line items
	const subtotal = lineItems.reduce((sum: number, li: TemplateLineItem) => sum + (li.amount ?? li.quantity * li.rate), 0);
	const taxAmount = subtotal * (template.tax_rate / 100);
	const total = subtotal + taxAmount;

	const defaultCurrency = profile?.default_currency ?? 'USD';

	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, payment_terms, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[
				crypto.randomUUID(),
				invoiceNumber,
				template.client_id,
				todayStr,
				todayStr,
				'custom',
				subtotal,
				template.tax_rate,
				taxAmount,
				total,
				template.notes ?? '',
				'draft',
				defaultCurrency,
				businessSnap,
				clientSnap,
				'{}'
			]
		);

		const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
		const invoiceId = result[0].id;

		for (let i = 0; i < lineItems.length; i++) {
			const li = lineItems[i];
			execute(
				`INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				[
					crypto.randomUUID(),
					invoiceId,
					li.description,
					li.quantity,
					li.rate,
					li.amount ?? li.quantity * li.rate,
					li.notes ?? '',
					li.sort_order ?? i
				]
			);
		}

		logAudit({
			entity_type: 'invoice',
			entity_id: invoiceId,
			action: 'create',
			context: `${invoiceNumber} (from recurring template: ${template.name})`
		});

		runRaw('COMMIT');

		// Advance template next_due
		const newDate = advanceNextDue(template.next_due, template.frequency);
		execute(
			`UPDATE recurring_templates SET next_due = ?, updated_at = datetime('now') WHERE id = ?`,
			[newDate, templateId]
		);

		return invoiceId;
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}
