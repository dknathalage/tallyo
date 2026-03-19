import type { CLIENT_COLUMNS, CATALOG_COLUMNS, INVOICE_COLUMNS, ESTIMATE_COLUMNS } from './columns.js';

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
	currencyCode: string;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	lineItems: {
		description: string;
		quantity: number;
		rate: number;
		amount: number;
		sortOrder: number;
		notes: string;
	}[];
	isNew: boolean;
}

export interface ParsedInvoiceImport extends ParsedImport<CsvInvoiceRow> {
	groups: ParsedInvoiceGroup[];
	newClientsToCreate: string[];
}

export type CsvEstimateRow = Record<(typeof ESTIMATE_COLUMNS)[number], string>;

export interface ParsedEstimateGroup {
	estimateUuid: string;
	estimateNumber: string;
	clientName: string;
	clientEmail: string;
	date: string;
	validUntil: string;
	taxRate: number;
	notes: string;
	status: string;
	currencyCode: string;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	lineItems: {
		description: string;
		quantity: number;
		rate: number;
		amount: number;
		sortOrder: number;
		notes: string;
	}[];
	isNew: boolean;
}

export interface ParsedEstimateImport extends ParsedImport<CsvEstimateRow> {
	groups: ParsedEstimateGroup[];
	newClientsToCreate: string[];
}
