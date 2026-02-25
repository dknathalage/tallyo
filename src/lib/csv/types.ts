import type { CLIENT_COLUMNS, CATALOG_COLUMNS, INVOICE_COLUMNS } from './columns.js';

export type CsvClientRow = Record<(typeof CLIENT_COLUMNS)[number], string>;
export type CsvCatalogRow = Record<(typeof CATALOG_COLUMNS)[number], string>;
export type CsvInvoiceRow = Record<(typeof INVOICE_COLUMNS)[number], string>;

export interface ValidationError {
	row: number;
	field: string;
	message: string;
}

export interface ParsedImport<T> {
	validRows: T[];
	errors: ValidationError[];
	skippedDuplicates: number;
	totalRows: number;
}

export interface ParsedInvoiceGroup {
	invoiceUuid: string;
	invoiceNumber: string;
	clientName: string;
	clientEmail: string;
	date: string;
	dueDate: string;
	taxRate: number;
	notes: string;
	status: string;
	lineItems: {
		description: string;
		quantity: number;
		rate: number;
		amount: number;
		sortOrder: number;
	}[];
	isNew: boolean;
}

export interface ParsedInvoiceImport extends ParsedImport<CsvInvoiceRow> {
	groups: ParsedInvoiceGroup[];
	newClientsToCreate: string[];
}
