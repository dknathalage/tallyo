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
