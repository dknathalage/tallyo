import { parseCsvFile, validateRequiredField, validateNumeric, validateDate } from './parse.js';
import type { CsvEstimateRow, ParsedEstimateImport, ParsedEstimateGroup, ValidationError } from './types.js';
import type { EstimateRepository, ClientRepository } from '$lib/repositories/interfaces/index.js';

function validateEstimateStatus(value: string, row: number): ValidationError | null {
	const valid = ['draft', 'sent', 'accepted', 'rejected', 'expired'];
	if (value && !valid.includes(value.toLowerCase())) {
		return { row, field: 'status', message: `status must be one of: ${valid.join(', ')}` };
	}
	return null;
}

function validateEstimateRow(row: CsvEstimateRow, rowNum: number): ValidationError[] {
	const errs: ValidationError[] = [];

	for (const field of ['estimate_number', 'client_name', 'date', 'line_description'] as const) {
		const err = validateRequiredField(row[field], field, rowNum);
		if (err) errs.push(err);
	}

	const dateErr = validateDate(row.date, 'date', rowNum);
	if (dateErr) errs.push(dateErr);

	const validUntilErr = validateDate(row.valid_until, 'valid_until', rowNum);
	if (validUntilErr) errs.push(validUntilErr);

	for (const field of ['tax_rate', 'line_quantity', 'line_rate', 'line_amount'] as const) {
		const err = validateNumeric(row[field], field, rowNum);
		if (err) errs.push(err);
	}

	const statusErr = validateEstimateStatus(row.status, rowNum);
	if (statusErr) errs.push(statusErr);

	return errs;
}

function validateEstimateRows(
	data: CsvEstimateRow[]
): { validatedRows: CsvEstimateRow[]; errors: ValidationError[] } {
	const errors: ValidationError[] = [];
	const validatedRows: CsvEstimateRow[] = [];
	for (let i = 0; i < data.length; i++) {
		const row = data[i];
		if (!row) continue;
		const rowErrors = validateEstimateRow(row, i + 1);
		if (rowErrors.length === 0) {
			validatedRows.push(row);
		} else {
			errors.push(...rowErrors);
		}
	}
	return { validatedRows, errors };
}

function groupRowsByEstimate(rows: CsvEstimateRow[]): Map<string, CsvEstimateRow[]> {
	const groupMap = new Map<string, CsvEstimateRow[]>();
	for (const row of rows) {
		const key = row.estimate_uuid.trim() || row.estimate_number.trim();
		if (!key) continue;
		const group = groupMap.get(key);
		if (group) {
			group.push(row);
		} else {
			groupMap.set(key, [row]);
		}
	}
	return groupMap;
}

async function fetchExistingEstimateContext(): Promise<{
	existingUuids: Set<string>;
	clientNameMap: Map<string, number>;
}> {
	const [estimatesRes, clientsRes] = await Promise.all([
		fetch('/api/estimates?limit=10000'),
		fetch('/api/clients?limit=10000')
	]);
	const estimatesBody = await estimatesRes.json();
	const clientsBody = await clientsRes.json();
	const existingEstimates = (estimatesBody.data ?? estimatesBody) as { uuid: string }[];
	const existingClients = (clientsBody.data ?? clientsBody) as { id: number; name: string }[];
	const existingUuids = new Set(existingEstimates.map((r) => r.uuid).filter(Boolean));
	const clientNameMap = new Map<string, number>();
	for (const c of existingClients) {
		clientNameMap.set(c.name.toLowerCase(), c.id);
	}
	return { existingUuids, clientNameMap };
}

function buildEstimateLineItems(rows: CsvEstimateRow[]): ParsedEstimateGroup['lineItems'] {
	return rows.map((r, idx) => ({
		description: r.line_description.trim(),
		quantity: Number(r.line_quantity) || 1,
		rate: Number(r.line_rate) || 0,
		amount: Number(r.line_amount) || 0,
		sortOrder: Number(r.line_sort_order) || idx,
		notes: r.line_notes.trim()
	}));
}

function buildEstimateGroup(rows: CsvEstimateRow[], first: CsvEstimateRow): ParsedEstimateGroup {
	const estimateUuid = first.estimate_uuid.trim();
	const validUntil = first.valid_until.trim() || first.date.trim();
	return {
		estimateUuid: estimateUuid || crypto.randomUUID(),
		estimateNumber: first.estimate_number.trim(),
		clientName: first.client_name.trim(),
		clientEmail: first.client_email.trim(),
		date: first.date.trim(),
		validUntil,
		taxRate: Number(first.tax_rate) || 0,
		notes: first.notes.trim(),
		status: first.status.trim().toLowerCase() || 'draft',
		currencyCode: first.currency_code.trim() || 'USD',
		businessSnapshot: first.business_snapshot.trim() || '{}',
		clientSnapshot: first.client_snapshot.trim() || '{}',
		payerSnapshot: first.payer_snapshot.trim() || '{}',
		lineItems: buildEstimateLineItems(rows),
		isNew: true
	};
}

export async function parseEstimatesCsv(file: File): Promise<ParsedEstimateImport> {
	const { data } = await parseCsvFile<CsvEstimateRow>(file);
	const totalRows = data.length;
	const { validatedRows, errors } = validateEstimateRows(data);
	const groupMap = groupRowsByEstimate(validatedRows);
	const { existingUuids, clientNameMap } = await fetchExistingEstimateContext();

	const groups: ParsedEstimateGroup[] = [];
	const newClientsSet = new Set<string>();
	const validRows: CsvEstimateRow[] = [];
	let skippedDuplicates = 0;

	for (const [, rows] of groupMap) {
		const first = rows[0];
		if (!first) continue;

		const estimateUuid = first.estimate_uuid.trim();
		if (estimateUuid && existingUuids.has(estimateUuid)) {
			skippedDuplicates++;
			continue;
		}

		const clientName = first.client_name.trim();
		if (clientName && !clientNameMap.has(clientName.toLowerCase())) {
			newClientsSet.add(clientName);
		}

		groups.push(buildEstimateGroup(rows, first));
		validRows.push(...rows);
	}

	return {
		validRows,
		errors,
		skippedDuplicates,
		totalRows,
		groups,
		newClientsToCreate: [...newClientsSet]
	};
}

/**
 * Commits the parsed estimate import through the repository layer.
 * The repository layer handles transactions and audit logging for each record.
 */
export async function commitEstimateImport(
	groups: ParsedEstimateGroup[],
	newClients: string[],
	repos: { estimates: EstimateRepository; clients: ClientRepository }
): Promise<void> {
	// Auto-create missing clients through the repository (audit-logged)
	for (const name of newClients) {
		await repos.clients.createClient({ name });
	}

	// Rebuild client name→id map after creating new clients
	const allClientsRes = await fetch('/api/clients?limit=10000');
	const allClientsBody = await allClientsRes.json();
	const allClients = (allClientsBody.data ?? allClientsBody) as { id: number; name: string }[];
	const clientMap = new Map<string, number>();
	for (const c of allClients) {
		clientMap.set(c.name.toLowerCase(), c.id);
	}

	for (const group of groups) {
		const clientId = clientMap.get(group.clientName.toLowerCase());
		if (!clientId) continue;

		// Calculate totals from line items
		const subtotal = group.lineItems.reduce((sum, li) => sum + li.amount, 0);
		const taxAmount = (subtotal * group.taxRate) / 100;
		const total = subtotal + taxAmount;

		// Route the write through the repository (handles transaction + audit)
		await repos.estimates.createEstimate(
			{
				uuid: group.estimateUuid,
				estimate_number: group.estimateNumber,
				client_id: clientId,
				date: group.date,
				valid_until: group.validUntil,
				subtotal,
				tax_rate: group.taxRate,
				tax_amount: taxAmount,
				total,
				notes: group.notes,
				status: group.status,
				currency_code: group.currencyCode,
				business_snapshot: group.businessSnapshot,
				client_snapshot: group.clientSnapshot,
				payer_snapshot: group.payerSnapshot
			},
			group.lineItems.map((li) => ({
				description: li.description,
				quantity: li.quantity,
				rate: li.rate,
				amount: li.amount,
				sort_order: li.sortOrder,
				notes: li.notes
			}))
		);
	}
}
