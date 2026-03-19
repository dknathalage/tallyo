import { parseCsvFile, validateRequiredField, validateNumeric, validateDate, validateStatus } from './parse.js';
import type { CsvInvoiceRow, ParsedInvoiceImport, ParsedInvoiceGroup, ValidationError } from './types.js';
import type { InvoiceRepository, ClientRepository } from '$lib/repositories/interfaces/index.js';

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

	// Check existing invoice UUIDs for deduplication via API
	const [invoicesRes, clientsRes] = await Promise.all([
		fetch('/api/invoices'),
		fetch('/api/clients')
	]);
	const existingInvoices = await invoicesRes.json() as Array<{ uuid: string }>;
	const existingClients = await clientsRes.json() as Array<{ id: number; name: string }>;
	const existingUuids = new Set(existingInvoices.map((r) => r.uuid).filter(Boolean));
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
			sortOrder: Number(r.line_sort_order) || idx,
			notes: r.line_notes?.trim() || ''
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
			currencyCode: first.currency_code?.trim() || 'USD',
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

/**
 * Commits the parsed invoice import through the repository layer.
 * The repository layer handles transactions and audit logging for each record.
 */
export async function commitInvoiceImport(
	groups: ParsedInvoiceGroup[],
	newClients: string[],
	repos: { invoices: InvoiceRepository; clients: ClientRepository }
): Promise<void> {
	// Auto-create missing clients through the repository (audit-logged)
	for (const name of newClients) {
		await repos.clients.createClient({ name });
	}

	// Rebuild client name→id map after creating new clients
	const allClientsRes = await fetch('/api/clients');
	const allClients = await allClientsRes.json() as Array<{ id: number; name: string }>;
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
		await repos.invoices.createInvoice(
			{
				uuid: group.invoiceUuid,
				invoice_number: group.invoiceNumber,
				client_id: clientId,
				date: group.date,
				due_date: group.dueDate,
				subtotal,
				tax_rate: group.taxRate,
				tax_amount: taxAmount,
				total,
				notes: group.notes,
				status: group.status,
				currency_code: group.currencyCode || 'USD',
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
				notes: li.notes ?? ''
			}))
		);
	}
}
