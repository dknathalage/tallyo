import { query, execute, runRaw, save } from '$lib/db/connection.svelte.js';
import { parseCsvFile, validateRequiredField, validateNumeric, validateDate } from './parse.js';
import type { CsvEstimateRow, ParsedEstimateImport, ParsedEstimateGroup, ValidationError } from './types.js';

function validateEstimateStatus(value: string | undefined, row: number): ValidationError | null {
	const valid = ['draft', 'sent', 'accepted', 'rejected', 'expired'];
	if (value && !valid.includes(value.toLowerCase())) {
		return { row, field: 'status', message: `status must be one of: ${valid.join(', ')}` };
	}
	return null;
}

export async function parseEstimatesCsv(file: File): Promise<ParsedEstimateImport> {
	const { data } = await parseCsvFile<CsvEstimateRow>(file);
	const totalRows = data.length;
	const errors: ValidationError[] = [];
	let skippedDuplicates = 0;

	// Validate each row
	const validatedRows: CsvEstimateRow[] = [];
	for (let i = 0; i < data.length; i++) {
		const row = data[i];
		const rowNum = i + 1;
		let hasError = false;

		// Required fields
		for (const field of ['estimate_number', 'client_name', 'date', 'line_description'] as const) {
			const err = validateRequiredField(row[field], field, rowNum);
			if (err) { errors.push(err); hasError = true; }
		}

		// Date validations
		const dateErr = validateDate(row.date, 'date', rowNum);
		if (dateErr) { errors.push(dateErr); hasError = true; }

		const validUntilErr = validateDate(row.valid_until, 'valid_until', rowNum);
		if (validUntilErr) { errors.push(validUntilErr); hasError = true; }

		// Numeric validations
		for (const field of ['tax_rate', 'line_quantity', 'line_rate', 'line_amount'] as const) {
			const err = validateNumeric(row[field], field, rowNum);
			if (err) { errors.push(err); hasError = true; }
		}

		// Status validation
		const statusErr = validateEstimateStatus(row.status, rowNum);
		if (statusErr) { errors.push(statusErr); hasError = true; }

		if (!hasError) validatedRows.push(row);
	}

	// Group rows into estimates
	const groupMap = new Map<string, CsvEstimateRow[]>();
	for (const row of validatedRows) {
		const key = row.estimate_uuid?.trim() || row.estimate_number?.trim() || '';
		if (!key) continue;
		const group = groupMap.get(key);
		if (group) {
			group.push(row);
		} else {
			groupMap.set(key, [row]);
		}
	}

	// Check existing estimate UUIDs for deduplication
	const existingEstimates = query<{ uuid: string }>('SELECT uuid FROM estimates WHERE uuid IS NOT NULL');
	const existingUuids = new Set(existingEstimates.map((r) => r.uuid));

	// Check existing clients for matching
	const existingClients = query<{ id: number; name: string }>('SELECT id, name FROM clients');
	const clientNameMap = new Map<string, number>();
	for (const c of existingClients) {
		clientNameMap.set(c.name.toLowerCase(), c.id);
	}

	const groups: ParsedEstimateGroup[] = [];
	const newClientsSet = new Set<string>();
	const validRows: CsvEstimateRow[] = [];

	for (const [, rows] of groupMap) {
		const first = rows[0];
		const estimateUuid = first.estimate_uuid?.trim() || '';

		// Skip groups whose UUID already exists
		if (estimateUuid && existingUuids.has(estimateUuid)) {
			skippedDuplicates++;
			continue;
		}

		const clientName = first.client_name?.trim() || '';

		// Track new clients that need to be created
		if (clientName && !clientNameMap.has(clientName.toLowerCase())) {
			newClientsSet.add(clientName);
		}

		const lineItems = rows.map((r, idx) => ({
			description: r.line_description?.trim() || '',
			quantity: Number(r.line_quantity) || 1,
			rate: Number(r.line_rate) || 0,
			amount: Number(r.line_amount) || 0,
			sortOrder: Number(r.line_sort_order) || idx,
			notes: r.line_notes?.trim() || ''
		}));

		groups.push({
			estimateUuid: estimateUuid || crypto.randomUUID(),
			estimateNumber: first.estimate_number?.trim() || '',
			clientName,
			clientEmail: first.client_email?.trim() || '',
			date: first.date?.trim() || '',
			validUntil: first.valid_until?.trim() || first.date?.trim() || '',
			taxRate: Number(first.tax_rate) || 0,
			notes: first.notes?.trim() || '',
			status: first.status?.trim().toLowerCase() || 'draft',
			currencyCode: (first as any).currency_code?.trim() || 'USD',
			businessSnapshot: first.business_snapshot?.trim() || '{}',
			clientSnapshot: first.client_snapshot?.trim() || '{}',
			payerSnapshot: first.payer_snapshot?.trim() || '{}',
			lineItems,
			isNew: true
		});

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

export async function commitEstimateImport(groups: ParsedEstimateGroup[], newClients: string[]): Promise<void> {
	try {
		runRaw('BEGIN TRANSACTION');

		// Auto-create missing clients
		for (const name of newClients) {
			execute(
				'INSERT INTO clients (uuid, name) VALUES (?, ?)',
				[crypto.randomUUID(), name]
			);
		}

		// Build client name->id map (case-insensitive)
		const allClients = query<{ id: number; name: string }>('SELECT id, name FROM clients');
		const clientMap = new Map<string, number>();
		for (const c of allClients) {
			clientMap.set(c.name.toLowerCase(), c.id);
		}

		for (const group of groups) {
			const clientId = clientMap.get(group.clientName.toLowerCase());
			if (!clientId) continue;

			// Handle duplicate estimate numbers
			let estimateNumber = group.estimateNumber;
			const dupes = query<{ id: number }>('SELECT id FROM estimates WHERE estimate_number = ?', [estimateNumber]);
			if (dupes.length > 0) {
				estimateNumber = `${estimateNumber}-imported-${Date.now()}`;
			}

			// Calculate totals from line items
			const subtotal = group.lineItems.reduce((sum, li) => sum + li.amount, 0);
			const taxAmount = subtotal * group.taxRate / 100;
			const total = subtotal + taxAmount;

			execute(
				'INSERT INTO estimates (uuid, estimate_number, client_id, date, valid_until, subtotal, tax_rate, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
				[
					group.estimateUuid,
					estimateNumber,
					clientId,
					group.date,
					group.validUntil,
					subtotal,
					group.taxRate,
					taxAmount,
					total,
					group.notes,
					group.status,
					group.currencyCode || 'USD',
					group.businessSnapshot,
					group.clientSnapshot,
					group.payerSnapshot
				]
			);

			// Get the inserted estimate's id
			const inserted = query<{ id: number }>('SELECT last_insert_rowid() as id');
			const estimateId = inserted[0]?.id;
			if (!estimateId) continue;

			// Insert line items
			for (const li of group.lineItems) {
				execute(
					'INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?)',
					[
						crypto.randomUUID(),
						estimateId,
						li.description,
						li.quantity,
						li.rate,
						li.amount,
						li.notes ?? '',
						li.sortOrder
					]
				);
			}
		}

		runRaw('COMMIT');
		await save();
	} catch (err) {
		runRaw('ROLLBACK');
		throw err;
	}
}
