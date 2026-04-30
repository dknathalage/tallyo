import { getDb } from '../connection.js';
import { estimates, estimateLineItems, invoices, lineItems, clients } from '../drizzle-schema.js';
import { eq, and, or, like, desc, sql, inArray } from 'drizzle-orm';
import { generateInvoiceNumber, generateEstimateNumber } from '../number-generators.js';
import type { Estimate, EstimateLineItem, PaginationParams, PaginatedResult } from '../../types/index.js';
import { paginate } from '../../types/index.js';
import type { CreateEstimateInput, UpdateEstimateInput, LineItemInput } from '../../repositories/interfaces/types.js';

function toISOString(d: string | null | undefined): string {
	if (!d) return '';
	return d;
}

function mapRowToEstimate(row: Record<string, unknown>): Estimate {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		estimate_number: row.estimate_number as string,
		client_id: row.client_id as number,
		client_name: (row.client_name as string) ?? undefined,
		date: row.date as string,
		valid_until: row.valid_until as string,
		subtotal: row.subtotal as number,
		tax_rate: row.tax_rate as number,
		tax_rate_id: (row.tax_rate_id as number | null) ?? null,
		tax_amount: row.tax_amount as number,
		total: row.total as number,
		notes: (row.notes as string) ?? '',
		status: row.status as Estimate['status'],
		currency_code: (row.currency_code as string) ?? 'USD',
		converted_invoice_id: (row.converted_invoice_id as number | null) ?? null,
		business_snapshot: (row.business_snapshot as string) ?? '{}',
		client_snapshot: (row.client_snapshot as string) ?? '{}',
		payer_snapshot: (row.payer_snapshot as string) ?? '{}',
		created_at: toISOString(row.created_at as string | null),
		updated_at: toISOString(row.updated_at as string | null)
	};
}

function mapRowToEstimateLineItem(row: Record<string, unknown>): EstimateLineItem {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		estimate_id: row.estimate_id as number,
		description: row.description as string,
		quantity: row.quantity as number,
		rate: row.rate as number,
		amount: row.amount as number,
		notes: (row.notes as string) ?? '',
		sort_order: (row.sort_order as number) ?? 0,
		catalog_item_id: (row.catalog_item_id as number | null) ?? null,
		rate_tier_id: (row.rate_tier_id as number | null) ?? null
	};
}

const estimateSelectFields = {
	id: estimates.id,
	uuid: estimates.uuid,
	estimate_number: estimates.estimate_number,
	client_id: estimates.client_id,
	client_name: clients.name,
	date: estimates.date,
	valid_until: estimates.valid_until,
	subtotal: estimates.subtotal,
	tax_rate: estimates.tax_rate,
	tax_rate_id: estimates.tax_rate_id,
	tax_amount: estimates.tax_amount,
	total: estimates.total,
	notes: estimates.notes,
	status: estimates.status,
	currency_code: estimates.currency_code,
	converted_invoice_id: estimates.converted_invoice_id,
	business_snapshot: estimates.business_snapshot,
	client_snapshot: estimates.client_snapshot,
	payer_snapshot: estimates.payer_snapshot,
	created_at: estimates.created_at,
	updated_at: estimates.updated_at
};

export async function getEstimates(
	search?: string,
	status?: string,
	pagination?: PaginationParams
): Promise<PaginatedResult<Estimate>> {
	const db = getDb();
	const conditions: ReturnType<typeof eq>[] = [];

	if (search) {
		conditions.push(
			or(
				like(estimates.estimate_number, `%${search}%`),
				like(clients.name, `%${search}%`)
			)!
		);
	}
	if (status) {
		conditions.push(eq(estimates.status, status));
	}

	const query = db
		.select(estimateSelectFields)
		.from(estimates)
		.leftJoin(clients, eq(estimates.client_id, clients.id))
		.orderBy(desc(estimates.created_at));

	const rows =
		conditions.length > 0
			? await query.where(and(...conditions))
			: await query;

	const all = rows.map((r) => mapRowToEstimate(r as unknown as Record<string, unknown>));
	return paginate(all, pagination);
}

export async function getEstimate(id: number): Promise<Estimate | null> {
	const db = getDb();
	const rows = await db
		.select(estimateSelectFields)
		.from(estimates)
		.leftJoin(clients, eq(estimates.client_id, clients.id))
		.where(eq(estimates.id, id));

	if (rows.length === 0) return null;
	return mapRowToEstimate(rows[0] as unknown as Record<string, unknown>);
}

export async function getEstimateLineItems(estimateId: number): Promise<EstimateLineItem[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(estimateLineItems)
		.where(eq(estimateLineItems.estimate_id, estimateId))
		.orderBy(estimateLineItems.sort_order);

	return rows.map((r) => mapRowToEstimateLineItem(r as unknown as Record<string, unknown>));
}

/**
 * Inserts the estimate and its line items in a transaction, returns the new estimate id.
 */
export async function createEstimate(
	data: CreateEstimateInput,
	items: LineItemInput[]
): Promise<number> {
	const db = getDb();
	return await db.transaction(async (tx) => {
		const [inserted] = await tx
			.insert(estimates)
			.values({
				uuid: data.uuid ?? crypto.randomUUID(),
				estimate_number: data.estimate_number,
				client_id: data.client_id,
				date: data.date,
				valid_until: data.valid_until,
				subtotal: data.subtotal,
				tax_rate: data.tax_rate,
				tax_rate_id: data.tax_rate_id ?? null,
				tax_amount: data.tax_amount,
				total: data.total,
				notes: data.notes ?? '',
				status: data.status ?? 'draft',
				currency_code: data.currency_code ?? 'USD',
				business_snapshot: data.business_snapshot ?? '{}',
				client_snapshot: data.client_snapshot ?? '{}',
				payer_snapshot: data.payer_snapshot ?? '{}'
			})
			.returning({ id: estimates.id });

		const estimateId = inserted.id;

		for (const item of items) {
			await tx.insert(estimateLineItems).values({
				uuid: crypto.randomUUID(),
				estimate_id: estimateId,
				description: item.description,
				quantity: item.quantity,
				rate: item.rate,
				amount: item.amount,
				notes: item.notes ?? '',
				sort_order: item.sort_order
			});
		}

		return estimateId;
	});
}

/**
 * Updates the estimate and replaces its line items.
 */
export async function updateEstimate(
	id: number,
	data: UpdateEstimateInput,
	items: LineItemInput[]
): Promise<void> {
	const db = getDb();

	await db
		.update(estimates)
		.set({
			estimate_number: data.estimate_number,
			client_id: data.client_id,
			date: data.date,
			valid_until: data.valid_until,
			subtotal: data.subtotal,
			tax_rate: data.tax_rate,
			tax_rate_id: data.tax_rate_id ?? null,
			tax_amount: data.tax_amount,
			total: data.total,
			notes: data.notes ?? '',
			status: data.status ?? 'draft',
			currency_code: data.currency_code ?? 'USD',
			business_snapshot: data.business_snapshot ?? '{}',
			client_snapshot: data.client_snapshot ?? '{}',
			payer_snapshot: data.payer_snapshot ?? '{}',
			updated_at: new Date().toISOString()
		})
		.where(eq(estimates.id, id));

	await db.delete(estimateLineItems).where(eq(estimateLineItems.estimate_id, id));

	for (const item of items) {
		await db.insert(estimateLineItems).values({
			uuid: crypto.randomUUID(),
			estimate_id: id,
			description: item.description,
			quantity: item.quantity,
			rate: item.rate,
			amount: item.amount,
			notes: item.notes ?? '',
			sort_order: item.sort_order
		});
	}
}

/**
 * Deletes the estimate.
 */
export async function deleteEstimate(id: number): Promise<void> {
	const db = getDb();
	await db.delete(estimates).where(eq(estimates.id, id));
}

/**
 * Updates estimate status.
 */
export async function updateEstimateStatus(id: number, status: string): Promise<void> {
	const db = getDb();
	await db
		.update(estimates)
		.set({ status, updated_at: new Date().toISOString() })
		.where(eq(estimates.id, id));
}

/**
 * Bulk deletes estimates and their line items.
 */
export async function bulkDeleteEstimates(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db.delete(estimateLineItems).where(inArray(estimateLineItems.estimate_id, ids));
	await db.delete(estimates).where(inArray(estimates.id, ids));
}

/**
 * Bulk updates estimate status.
 */
export async function bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db
		.update(estimates)
		.set({ status, updated_at: new Date().toISOString() })
		.where(inArray(estimates.id, ids));
}

export async function getClientEstimates(clientId: number): Promise<Estimate[]> {
	const db = getDb();
	const rows = await db
		.select(estimateSelectFields)
		.from(estimates)
		.leftJoin(clients, eq(estimates.client_id, clients.id))
		.where(eq(estimates.client_id, clientId))
		.orderBy(desc(estimates.created_at));

	return rows.map((r) => mapRowToEstimate(r as unknown as Record<string, unknown>));
}

/**
 * Converts an accepted estimate into an invoice.
 * Returns an object with both the new invoice id and info needed for audit.
 */
export async function convertEstimateToInvoice(
	estimateId: number
): Promise<{ invoiceId: number; invoiceNumber: string; estimateNumber: string }> {
	const estimate = await getEstimate(estimateId);
	if (!estimate) throw new Error('Estimate not found');
	if (estimate.status !== 'accepted')
		throw new Error('Only accepted estimates can be converted to invoices');
	if (estimate.converted_invoice_id !== null)
		throw new Error('Estimate has already been converted to an invoice');

	const items = await getEstimateLineItems(estimateId);
	const invoiceNumber = await generateInvoiceNumber();

	const db = getDb();
	const [inserted] = await db
		.insert(invoices)
		.values({
			uuid: crypto.randomUUID(),
			invoice_number: invoiceNumber,
			client_id: estimate.client_id,
			date: estimate.date,
			due_date: estimate.valid_until,
			subtotal: estimate.subtotal,
			tax_rate: estimate.tax_rate,
			tax_amount: estimate.tax_amount,
			total: estimate.total,
			notes: estimate.notes,
			status: 'draft',
			currency_code: estimate.currency_code,
			business_snapshot: estimate.business_snapshot,
			client_snapshot: estimate.client_snapshot,
			payer_snapshot: estimate.payer_snapshot
		})
		.returning({ id: invoices.id });

	const invoiceId = inserted.id;

	for (const item of items) {
		await db.insert(lineItems).values({
			uuid: crypto.randomUUID(),
			invoice_id: invoiceId,
			description: item.description,
			quantity: item.quantity,
			rate: item.rate,
			amount: item.amount,
			notes: item.notes ?? '',
			sort_order: item.sort_order
		});
	}

	await db
		.update(estimates)
		.set({ converted_invoice_id: invoiceId, updated_at: new Date().toISOString() })
		.where(eq(estimates.id, estimateId));

	return { invoiceId, invoiceNumber, estimateNumber: estimate.estimate_number };
}

/**
 * Duplicates an estimate and its line items.
 * Returns the new estimate id and its number.
 */
export async function duplicateEstimate(
	id: number
): Promise<{ newId: number; newNumber: string; originalNumber: string }> {
	const original = await getEstimate(id);
	if (!original) throw new Error(`Estimate ${id} not found`);

	const newNumber = await generateEstimateNumber();
	const todayStr = new Date().toISOString().slice(0, 10);

	const originalItems = await getEstimateLineItems(id);

	const db = getDb();
	return await db.transaction(async (tx) => {
		const [inserted] = await tx
			.insert(estimates)
			.values({
				uuid: crypto.randomUUID(),
				estimate_number: newNumber,
				client_id: original.client_id,
				date: todayStr,
				valid_until: '',
				subtotal: original.subtotal,
				tax_rate: original.tax_rate,
				tax_amount: original.tax_amount,
				total: original.total,
				notes: original.notes ?? '',
				status: 'draft',
				currency_code: original.currency_code ?? 'USD',
				business_snapshot: original.business_snapshot ?? '{}',
				client_snapshot: original.client_snapshot ?? '{}',
				payer_snapshot: original.payer_snapshot ?? '{}'
			})
			.returning({ id: estimates.id });

		const newId = inserted.id;

		for (const item of originalItems) {
			await tx.insert(estimateLineItems).values({
				uuid: crypto.randomUUID(),
				estimate_id: newId,
				description: item.description,
				quantity: item.quantity,
				rate: item.rate,
				amount: item.amount,
				notes: item.notes ?? '',
				sort_order: item.sort_order
			});
		}

		return { newId, newNumber, originalNumber: original.estimate_number };
	});
}
