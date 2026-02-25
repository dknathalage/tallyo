import { query, execute, runRaw, save } from '$lib/db/connection.svelte.js';
import { parseCsvFile, validateRequiredField, validateNumeric, validateDate, validateStatus } from './parse.js';
import type { CsvInvoiceRow, ParsedInvoiceImport, ParsedInvoiceGroup, ValidationError } from './types.js';

export async function parseInvoicesCsv(file: File): Promise<ParsedInvoiceImport> {
	const { data } = await parseCsvFile<CsvInvoiceRow>(file);
	const totalRows = data.length;
	const errors: ValidationError[] = [];
	let skippedDuplicates = 0;

	// Validate each row
	const validatedRows: CsvInvoiceRow[] = [];
	for (let i = 0; i < data.length; i++) {
		const row = data[i];
		const rowNum = i + 1;
		let hasError = false;

		// Required fields
		for (const field of ['invoice_number', 'client_name', 'date', 'line_description'] as const) {
			const err = validateRequiredField(row[field], field, rowNum);
			if (err) { errors.push(err); hasError = true; }
		}

		// Date validations
		const dateErr = validateDate(row.date, 'date', rowNum);
		if (dateErr) { errors.push(dateErr); hasError = true; }

		const dueDateErr = validateDate(row.due_date, 'due_date', rowNum);
		if (dueDateErr) { errors.push(dueDateErr); hasError = true; }

		// Numeric validations
		for (const field of ['tax_rate', 'line_quantity', 'line_rate', 'line_amount'] as const) {
			const err = validateNumeric(row[field], field, rowNum);
			if (err) { errors.push(err); hasError = true; }
		}

		// Status validation
		const statusErr = validateStatus(row.status, rowNum);
		if (statusErr) { errors.push(statusErr); hasError = true; }

		if (!hasError) validatedRows.push(row);
	}

	// Group rows into invoices
	const groupMap = new Map<string, CsvInvoiceRow[]>();
	for (const row of validatedRows) {
		const key = row.invoice_uuid?.trim() || row.invoice_number?.trim() || '';
		if (!key) continue;
		const group = groupMap.get(key);
		if (group) {
			group.push(row);
		} else {
			groupMap.set(key, [row]);
		}
	}

	// Check existing invoice UUIDs for deduplication
	const existingInvoices = query<{ uuid: string }>('SELECT uuid FROM invoices WHERE uuid IS NOT NULL');
	const existingUuids = new Set(existingInvoices.map((r) => r.uuid));

	// Check existing clients for matching
	const existingClients = query<{ id: number; name: string }>('SELECT id, name FROM clients');
	const clientNameMap = new Map<string, number>();
	for (const c of existingClients) {
		clientNameMap.set(c.name.toLowerCase(), c.id);
	}

	const groups: ParsedInvoiceGroup[] = [];
	const newClientsSet = new Set<string>();
	const validRows: CsvInvoiceRow[] = [];

	for (const [, rows] of groupMap) {
		const first = rows[0];
		const invoiceUuid = first.invoice_uuid?.trim() || '';

		// Skip groups whose UUID already exists
		if (invoiceUuid && existingUuids.has(invoiceUuid)) {
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
			sortOrder: Number(r.line_sort_order) || idx
		}));

		groups.push({
			invoiceUuid: invoiceUuid || crypto.randomUUID(),
			invoiceNumber: first.invoice_number?.trim() || '',
			clientName,
			clientEmail: first.client_email?.trim() || '',
			date: first.date?.trim() || '',
			dueDate: first.due_date?.trim() || first.date?.trim() || '',
			taxRate: Number(first.tax_rate) || 0,
			notes: first.notes?.trim() || '',
			status: first.status?.trim().toLowerCase() || 'draft',
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

export async function commitInvoiceImport(groups: ParsedInvoiceGroup[], newClients: string[]): Promise<void> {
	try {
		runRaw('BEGIN TRANSACTION');

		// Auto-create missing clients
		for (const name of newClients) {
			execute(
				'INSERT INTO clients (uuid, name) VALUES (?, ?)',
				[crypto.randomUUID(), name]
			);
		}

		// Build client name→id map (case-insensitive)
		const allClients = query<{ id: number; name: string }>('SELECT id, name FROM clients');
		const clientMap = new Map<string, number>();
		for (const c of allClients) {
			clientMap.set(c.name.toLowerCase(), c.id);
		}

		for (const group of groups) {
			const clientId = clientMap.get(group.clientName.toLowerCase());
			if (!clientId) continue;

			// Handle duplicate invoice numbers
			let invoiceNumber = group.invoiceNumber;
			const dupes = query<{ id: number }>('SELECT id FROM invoices WHERE invoice_number = ?', [invoiceNumber]);
			if (dupes.length > 0) {
				invoiceNumber = `${invoiceNumber}-imported-${Date.now()}`;
			}

			// Calculate totals from line items
			const subtotal = group.lineItems.reduce((sum, li) => sum + li.amount, 0);
			const taxAmount = subtotal * group.taxRate / 100;
			const total = subtotal + taxAmount;

			execute(
				'INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, subtotal, tax_rate, tax_amount, total, notes, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)',
				[
					group.invoiceUuid,
					invoiceNumber,
					clientId,
					group.date,
					group.dueDate,
					subtotal,
					group.taxRate,
					taxAmount,
					total,
					group.notes,
					group.status
				]
			);

			// Get the inserted invoice's id
			const inserted = query<{ id: number }>('SELECT last_insert_rowid() as id');
			const invoiceId = inserted[0]?.id;
			if (!invoiceId) continue;

			// Insert line items
			for (const li of group.lineItems) {
				execute(
					'INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)',
					[
						crypto.randomUUID(),
						invoiceId,
						li.description,
						li.quantity,
						li.rate,
						li.amount,
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
