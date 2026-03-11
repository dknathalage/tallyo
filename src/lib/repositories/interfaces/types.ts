// Input/output types for repository operations.
// These are plain TypeScript types with no sql.js dependencies.

export interface LineItemInput {
	description: string;
	quantity: number;
	rate: number;
	amount: number;
	sort_order: number;
	notes?: string;
}

export interface CreateInvoiceInput {
	invoice_number: string;
	client_id: number;
	date: string;
	due_date: string;
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
}

export type UpdateInvoiceInput = CreateInvoiceInput;

export interface CreateEstimateInput {
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
}

export type UpdateEstimateInput = CreateEstimateInput;

export interface CreateClientInput {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	pricing_tier_id?: number | null;
	metadata?: string;
	payer_id?: number | null;
}

export type UpdateClientInput = CreateClientInput;

export interface CreatePayerInput {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	metadata?: string;
}

export type UpdatePayerInput = CreatePayerInput;

export interface CreateCatalogItemInput {
	name: string;
	rate?: number;
	unit?: string;
	category?: string;
	sku?: string;
}

export type UpdateCatalogItemInput = CreateCatalogItemInput;

export interface CreateRateTierInput {
	name: string;
	description?: string;
	sort_order?: number;
}

export type UpdateRateTierInput = CreateRateTierInput;

export interface SaveBusinessProfileInput {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	logo?: string;
	metadata?: string;
	default_currency?: string;
}
