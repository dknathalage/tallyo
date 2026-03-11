import { execute, query, save, runRaw } from '../connection.svelte.js';
import { logAudit, computeChanges } from '../audit.js';
import { generateInvoiceNumber } from '../../utils/invoice-number.js';
import type { Estimate, EstimateLineItem } from '../../types/index.js';

export function getEstimates(search?: string, status?: string): Estimate[] {
	let sql = `SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id`;
	const params: unknown[] = [];
	const conditions: string[] = [];

	if (search) {
		conditions.push(`(e.estimate_number LIKE ? OR c.name LIKE ?)`);
		params.push(`%${search}%`, `%${search}%`);
	}
	if (status) {
		conditions.push(`e.status = ?`);
		params.push(status);
	}

	if (conditions.length > 0) {
		sql += ` WHERE ` + conditions.join(' AND ');
	}

	sql += ` ORDER BY e.created_at DESC`;
	return query<Estimate>(sql, params);
}

export function getEstimate(id: number): Estimate | null {
	const results = query<Estimate>(
		`SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.id = ?`,
		[id]
	);
	return results.length > 0 ? results[0] : null;
}

export function getEstimateLineItems(estimateId: number): EstimateLineItem[] {
	return query<EstimateLineItem>(
		`SELECT * FROM estimate_line_items WHERE estimate_id = ? ORDER BY sort_order`,
		[estimateId]
	);
}

export async function createEstimate(
	data: {
		estimate_number: string;
		client_id: number;
		date: string;
		valid_until: string;
		subtotal: number;
		tax_rate: number;
		tax_amount: number;
		total: number;
		notes?: string;
		status?: string;
		currency_code?: string;
		business_snapshot?: string;
		client_snapshot?: string;
		payer_snapshot?: string;
	},
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number; notes?: string }>
): Promise<number> {
	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`INSERT INTO estimates (uuid, estimate_number, client_id, date, valid_until, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[
				crypto.randomUUID(),
				data.estimate_number,
				data.client_id,
				data.date,
				data.valid_until,
				data.subtotal,
				data.tax_rate,
				data.tax_amount,
				data.total,
				data.notes ?? '',
				data.status ?? 'draft',
				data.currency_code ?? 'USD',
				data.business_snapshot ?? '{}',
				data.client_snapshot ?? '{}',
				data.payer_snapshot ?? '{}'
			]
		);

		const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
		const estimateId = result[0].id;

		for (const item of lineItems) {
			execute(
				`INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				[crypto.randomUUID(), estimateId, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
			);
		}

		logAudit({
			entity_type: 'estimate',
			entity_id: estimateId,
			action: 'create',
			context: data.estimate_number
		});

		runRaw('COMMIT');
		await save();
		return estimateId;
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function updateEstimate(
	id: number,
	data: {
		estimate_number: string;
		client_id: number;
		date: string;
		valid_until: string;
		subtotal: number;
		tax_rate: number;
		tax_amount: number;
		total: number;
		notes?: string;
		status?: string;
		currency_code?: string;
		business_snapshot?: string;
		client_snapshot?: string;
		payer_snapshot?: string;
	},
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number; notes?: string }>
): Promise<void> {
	const oldEstimate = getEstimate(id);
	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`UPDATE estimates SET estimate_number = ?, client_id = ?, date = ?, valid_until = ?, subtotal = ?, tax_rate = ?, tax_amount = ?, total = ?, notes = ?, status = ?, currency_code = ?, business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = datetime('now') WHERE id = ?`,
			[
				data.estimate_number,
				data.client_id,
				data.date,
				data.valid_until,
				data.subtotal,
				data.tax_rate,
				data.tax_amount,
				data.total,
				data.notes ?? '',
				data.status ?? 'draft',
				data.currency_code ?? 'USD',
				data.business_snapshot ?? '{}',
				data.client_snapshot ?? '{}',
				data.payer_snapshot ?? '{}',
				id
			]
		);

		execute(`DELETE FROM estimate_line_items WHERE estimate_id = ?`, [id]);

		for (const item of lineItems) {
			execute(
				`INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				[crypto.randomUUID(), id, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
			);
		}

		if (oldEstimate) {
			const changes = computeChanges(
				oldEstimate as unknown as Record<string, unknown>,
				{ ...data, notes: data.notes ?? '', status: data.status ?? 'draft', currency_code: data.currency_code ?? 'USD' },
				['estimate_number', 'client_id', 'date', 'valid_until', 'subtotal', 'tax_rate', 'total', 'notes', 'status', 'currency_code']
			);
			if (Object.keys(changes).length > 0) {
				logAudit({ entity_type: 'estimate', entity_id: id, action: 'update', changes, context: data.estimate_number });
			}
		}

		runRaw('COMMIT');
		await save();
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function deleteEstimate(id: number): Promise<void> {
	const estimate = getEstimate(id);
	runRaw('BEGIN TRANSACTION');
	try {
		execute(`DELETE FROM estimates WHERE id = ?`, [id]);
		logAudit({ entity_type: 'estimate', entity_id: id, action: 'delete', context: estimate?.estimate_number ?? '' });
		runRaw('COMMIT');
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
	await save();
}

export async function updateEstimateStatus(id: number, status: string): Promise<void> {
	const oldEstimate = getEstimate(id);
	execute(
		`UPDATE estimates SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		[status, id]
	);
	logAudit({
		entity_type: 'estimate',
		entity_id: id,
		action: 'status_change',
		changes: { status: { old: oldEstimate?.status ?? '', new: status } },
		context: oldEstimate?.estimate_number ?? ''
	});
	await save();
}

export async function bulkDeleteEstimates(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const batch_id = crypto.randomUUID();
	const estimates = ids.map((id) => getEstimate(id));
	const placeholders = ids.map(() => '?').join(',');
	runRaw('BEGIN TRANSACTION');
	try {
		execute(`DELETE FROM estimate_line_items WHERE estimate_id IN (${placeholders})`, ids);
		execute(`DELETE FROM estimates WHERE id IN (${placeholders})`, ids);
		for (let i = 0; i < ids.length; i++) {
			logAudit({ entity_type: 'estimate', entity_id: ids[i], action: 'delete', context: estimates[i]?.estimate_number ?? '', batch_id });
		}
		runRaw('COMMIT');
		await save();
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}

export async function bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void> {
	if (ids.length === 0) return;
	const batch_id = crypto.randomUUID();
	const estimates = ids.map((id) => getEstimate(id));
	const placeholders = ids.map(() => '?').join(',');
	execute(
		`UPDATE estimates SET status = ?, updated_at = datetime('now') WHERE id IN (${placeholders})`,
		[status, ...ids]
	);
	for (let i = 0; i < ids.length; i++) {
		logAudit({
			entity_type: 'estimate',
			entity_id: ids[i],
			action: 'status_change',
			changes: { status: { old: estimates[i]?.status ?? '', new: status } },
			context: estimates[i]?.estimate_number ?? '',
			batch_id
		});
	}
	await save();
}

export function getClientEstimates(clientId: number): Estimate[] {
	return query<Estimate>(
		`SELECT e.*, c.name as client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.client_id = ? ORDER BY e.created_at DESC`,
		[clientId]
	);
}

export async function convertEstimateToInvoice(estimateId: number): Promise<number> {
	const estimate = getEstimate(estimateId);
	if (!estimate) throw new Error('Estimate not found');
	if (estimate.status !== 'accepted') throw new Error('Only accepted estimates can be converted to invoices');
	if (estimate.converted_invoice_id !== null) throw new Error('Estimate has already been converted to an invoice');

	const lineItems = getEstimateLineItems(estimateId);
	const invoiceNumber = generateInvoiceNumber();

	runRaw('BEGIN TRANSACTION');
	try {
		execute(
			`INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[
				crypto.randomUUID(),
				invoiceNumber,
				estimate.client_id,
				estimate.date,
				estimate.valid_until,
				estimate.subtotal,
				estimate.tax_rate,
				estimate.tax_amount,
				estimate.total,
				estimate.notes,
				'draft',
				estimate.currency_code,
				estimate.business_snapshot,
				estimate.client_snapshot,
				estimate.payer_snapshot
			]
		);

		const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
		const invoiceId = result[0].id;

		for (const item of lineItems) {
			execute(
				`INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				[crypto.randomUUID(), invoiceId, item.description, item.quantity, item.rate, item.amount, item.notes ?? '', item.sort_order]
			);
		}

		execute(
			`UPDATE estimates SET converted_invoice_id = ?, updated_at = datetime('now') WHERE id = ?`,
			[invoiceId, estimateId]
		);

		logAudit({
			entity_type: 'estimate',
			entity_id: estimateId,
			action: 'convert_to_invoice',
			context: `${estimate.estimate_number} -> ${invoiceNumber}`
		});

		logAudit({
			entity_type: 'invoice',
			entity_id: invoiceId,
			action: 'create',
			context: `${invoiceNumber} (from estimate ${estimate.estimate_number})`
		});

		runRaw('COMMIT');
		await save();
		return invoiceId;
	} catch (e) {
		runRaw('ROLLBACK');
		throw e;
	}
}
