export type Role = 'owner' | 'admin' | 'member' | string;

export interface User {
	id: number;
	uuid: string;
	email: string;
	role: Role;
	lastLoginAt: string | null;
}

export interface SetupStatus {
	ownerExists: boolean;
}

export interface InviteInfo {
	email: string;
	role: Role;
}

export interface InviteCreated {
	token: string;
	acceptUrl: string;
}

export interface RateTier {
	id: number;
	uuid: string;
	name: string;
	description: string;
	sortOrder: number;
	createdAt: string;
	updatedAt: string;
}

export interface RateTierInput {
	name: string;
	description: string;
	sortOrder: number;
}

export interface Payer {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface PayerInput {
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
}

export interface TaxRate {
	id: number;
	uuid: string;
	name: string;
	rate: number;
	isDefault: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface TaxRateInput {
	name: string;
	rate: number;
	isDefault: boolean;
}

export interface Client {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	pricingTierId: number | null;
	pricingTierName: string;
	metadata: string;
	payerId: number | null;
	payerName: string;
	createdAt: string;
	updatedAt: string;
}

export interface ClientInput {
	name: string;
	email: string;
	phone: string;
	address: string;
	pricingTierId: number | null;
	metadata: string;
	payerId: number | null;
}

export interface CatalogItem {
	id: number;
	uuid: string;
	name: string;
	rate: number;
	unit: string;
	category: string;
	sku: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface CatalogItemInput {
	name: string;
	rate: number;
	unit: string;
	category: string;
	sku: string;
	metadata: string;
}

export interface LineItem {
	id: number;
	uuid: string;
	description: string;
	quantity: number;
	rate: number;
	amount: number;
	notes: string;
	sortOrder: number;
	catalogItemId: number | null;
	rateTierId: number | null;
}

export interface LineItemInput {
	description: string;
	quantity: number;
	rate: number;
	notes: string;
	sortOrder: number;
}

export type InvoiceStatus = 'draft' | 'sent' | 'overdue' | 'paid' | string;

export interface Invoice {
	id: number;
	uuid: string;
	invoiceNumber: string;
	clientId: number;
	clientName: string;
	date: string;
	dueDate: string;
	paymentTerms: string;
	subtotal: number;
	taxRate: number;
	taxRateId: number | null;
	taxAmount: number;
	total: number;
	totalPaid: number;
	balance: number;
	notes: string;
	status: InvoiceStatus;
	currencyCode: string;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	createdAt: string;
	updatedAt: string;
	lineItems: LineItem[];
}

export interface Payment {
	id: number;
	uuid: string;
	invoiceId: number;
	amount: number;
	paymentDate: string;
	method: string;
	notes: string;
	createdAt: string;
	updatedAt: string;
}

export interface PaymentInput {
	amount: number;
	paymentDate: string;
	method: string;
	notes: string;
}

export interface InvoiceInput {
	clientId: number;
	date: string;
	dueDate: string;
	paymentTerms: string;
	taxRate: number;
	taxRateId: number | null;
	notes: string;
	status: InvoiceStatus;
	currencyCode: string;
	lineItems: LineItemInput[];
}

export type EstimateStatus = 'draft' | 'accepted' | 'declined' | 'converted' | string;

// EstimateLineItem has the same shape as LineItem.
export type EstimateLineItem = LineItem;

// EstimateLineItemInput is identical to LineItemInput.
export type EstimateLineItemInput = LineItemInput;

export interface Estimate {
	id: number;
	uuid: string;
	estimateNumber: string;
	clientId: number;
	clientName: string;
	date: string;
	validUntil: string;
	subtotal: number;
	taxRate: number;
	taxRateId: number | null;
	taxAmount: number;
	total: number;
	notes: string;
	status: EstimateStatus;
	currencyCode: string;
	convertedInvoiceId: number | null;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	createdAt: string;
	updatedAt: string;
	lineItems: EstimateLineItem[];
}

export interface EstimateInput {
	clientId: number;
	date: string;
	validUntil: string;
	taxRate: number;
	taxRateId: number | null;
	notes: string;
	status: EstimateStatus;
	currencyCode: string;
	lineItems: EstimateLineItemInput[];
}

export interface ImportSuggestion {
	fields: Record<string, string>;
	baseHeader: string;
	priceCols: { header: string; suggestName: string }[];
}

export interface ImportParseResult {
	headers: string[];
	sample: Record<string, string>[];
	suggestion: ImportSuggestion;
}

export interface MappedRow {
	name: string;
	sku: string;
	unit: string;
	category: string;
	rate: number;
	metadata?: Record<string, string>;
	tierRates?: Record<string, number>;
}

export interface DiffResult {
	new: MappedRow[];
	updated: { existing: unknown; incoming: MappedRow }[];
	unchangedCount: number;
	summary: {
		total: number;
		new: number;
		updated: number;
		unchanged: number;
		errors: number;
	};
}

export interface CommitResult {
	inserted: number;
	updated: number;
	batchId: string;
}

export type RecurringFrequency = 'weekly' | 'monthly' | 'quarterly' | string;

export interface RecurringLine {
	description: string;
	quantity: number;
	rate: number;
	notes: string;
	sortOrder: number;
}

export interface RecurringTemplate {
	id: number;
	uuid: string;
	clientId: number | null;
	clientName: string;
	name: string;
	frequency: RecurringFrequency;
	nextDue: string;
	lineItems: RecurringLine[];
	taxRate: number;
	notes: string;
	isActive: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface RecurringInput {
	clientId: number;
	name: string;
	frequency: RecurringFrequency;
	nextDue: string;
	lineItems: RecurringLine[];
	taxRate: number;
	notes: string;
	isActive: boolean;
}
