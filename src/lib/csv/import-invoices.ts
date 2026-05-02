import { parseCsvFile, validateRequiredField, validateNumeric, validateDate, validateStatus } from './parse.js';
import type { CsvInvoiceRow, ParsedInvoiceImport, ParsedInvoiceGroup, ValidationError } from './types.js';
import type { InvoiceRepository, ClientRepository } from '$lib/repositories/interfaces/index.js';

function validateInvoiceRow(row: CsvInvoiceRow, rowNum: number): ValidationError[] {
	const errs: ValidationError[] = [];

	for (const field of ['invoice_number', 'client_name', 'date', 'line_description'] as const) {
		const err = validateRequiredField(row[field], field, rowNum);
		if (err) errs.push(err);
	}

	const dateErr = validateDate(row.date, 'date', rowNum);
	if (dateErr) errs.push(dateErr);

	const dueDateErr = validateDate(row.due_date, 'due_date', rowNum);
	if (dueDateErr) errs.push(dueDateErr);

	for (const field of ['tax_rate', 'line_quantity', 'line_rate', 'line_amount'] as const) {
		const err = validateNumeric(row[field], field, rowNum);
		if (err) errs.push(err);
	}

	const statusErr = validateStatus(row.status, rowNum);
	if (statusErr) errs.push(statusErr);

	return errs;
}

function validateInvoiceRows(
	data: CsvInvoiceRow[]
): { validatedRows: CsvInvoiceRow[]; errors: ValidationError[] } {
	const errors: ValidationError[] = [];
	const validatedRows: CsvInvoiceRow[] = [];
	for (let i = 0; i < data.length; i++) {
		const row = data[i];
		if (!row) continue;
		const rowErrors = validateInvoiceRow(row, i + 1);
		if (rowErrors.length === 0) {
			validatedRows.push(row);
		} else {
			errors.push(...rowErrors);
		}
	}
	return { validatedRows, errors };
}

function groupRowsByInvoice(rows: CsvInvoiceRow[]): Map<string, CsvInvoiceRow[]> {
	const groupMap = new Map<string, CsvInvoiceRow[]>();
	for (const row of rows) {
		const key = row.invoice_uuid.trim() || row.invoice_number.trim();
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

async function fetchExistingInvoiceContext(): Promise<{
	existingUuids: Set<string>;
	clientNameMap: Map<string, number>;
}> {
	const [invoicesRes, clientsRes] = await Promise.all([
		fetch('/api/invoices?limit=10000'),
		fetch('/api/clients?limit=10000')
	]);
	const invoicesBody = await invoicesRes.json();
	const clientsBody = await clientsRes.json();
	const existingInvoices = (invoicesBody.data ?? invoicesBody) as { uuid: string }[];
	const existingClients = (clientsBody.data ?? clientsBody) as { id: number; name: string }[];
	const existingUuids = new Set(existingInvoices.map((r) => r.uuid).filter(Boolean));
	const clientNameMap = new Map<string, number>();
	for (const c of existingClients) {
		clientNameMap.set(c.name.toLowerCase(), c.id);
	}
	return { existingUuids, clientNameMap };
}

function buildLineItems(rows: CsvInvoiceRow[]): ParsedInvoiceGroup['lineItems'] {
	return rows.map((r, idx) => ({
		description: r.line_description.trim(),
		quantity: Number(r.line_quantity) || 1,
		rate: Number(r.line_rate) || 0,
		amount: Number(r.line_amount) || 0,
		sortOrder: Number(r.line_sort_order) || idx,
		notes: r.line_notes.trim()
	}));
}

function buildInvoiceGroup(rows: CsvInvoiceRow[], first: CsvInvoiceRow): ParsedInvoiceGroup {
	const invoiceUuid = first.invoice_uuid.trim();
	const dueDate = first.due_date.trim() || first.date.trim();
	return {
		invoiceUuid: invoiceUuid || crypto.randomUUID(),
		invoiceNumber: first.invoice_number.trim(),
		clientName: first.client_name.trim(),
		clientEmail: first.client_email.trim(),
		date: first.date.trim(),
		dueDate,
		taxRate: Number(first.tax_rate) || 0,
		notes: first.notes.trim(),
		status: first.status.trim().toLowerCase() || 'draft',
		currencyCode: first.currency_code.trim() || 'USD',
		businessSnapshot: first.business_snapshot.trim() || '{}',
		clientSnapshot: first.client_snapshot.trim() || '{}',
		payerSnapshot: first.payer_snapshot.trim() || '{}',
		lineItems: buildLineItems(rows),
		isNew: true
	};
}

export async function parseInvoicesCsv(file: File): Promise<ParsedInvoiceImport> {
	const { data } = await parseCsvFile<CsvInvoiceRow>(file);
	const totalRows = data.length;
	const { validatedRows, errors } = validateInvoiceRows(data);
	const groupMap = groupRowsByInvoice(validatedRows);
	const { existingUuids, clientNameMap } = await fetchExistingInvoiceContext();

	const groups: ParsedInvoiceGroup[] = [];
	const newClientsSet = new Set<string>();
	const validRows: CsvInvoiceRow[] = [];
	let skippedDuplicates = 0;

	for (const [, rows] of groupMap) {
		const first = rows[0];
		if (!first) continue;

		const invoiceUuid = first.invoice_uuid.trim();
		if (invoiceUuid && existingUuids.has(invoiceUuid)) {
			skippedDuplicates++;
			continue;
		}

		const clientName = first.client_name.trim();
		if (clientName && !clientNameMap.has(clientName.toLowerCase())) {
			newClientsSet.add(clientName);
		}

		groups.push(buildInvoiceGroup(rows, first));
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
