export interface Client {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	pricing_tier_id: number | null;
	pricing_tier_name?: string;
	metadata: string;
	payer_id: number | null;
	payer_name?: string;
	created_at: string;
	updated_at: string;
}

export type PaymentTerms = 'due_on_receipt' | 'net_7' | 'net_14' | 'net_30' | 'net_60' | 'net_90' | 'custom';

export interface Invoice {
	id: number;
	uuid: string;
	invoice_number: string;
	client_id: number;
	client_name?: string;
	date: string;
	due_date: string;
	payment_terms: PaymentTerms;
	subtotal: number;
	tax_rate: number;
	tax_rate_id: number | null;
	tax_amount: number;
	total: number;
	notes: string;
	status: 'draft' | 'sent' | 'paid' | 'overdue';
	currency_code: string;
	business_snapshot: string;
	client_snapshot: string;
	payer_snapshot: string;
	created_at: string;
	updated_at: string;
}

export interface LineItem {
	id: number;
	uuid: string;
	invoice_id: number;
	description: string;
	quantity: number;
	rate: number;
	amount: number;
	notes: string;
	sort_order: number;
	catalog_item_id: number | null;
	rate_tier_id: number | null;
}

export type EstimateStatus = 'draft' | 'sent' | 'accepted' | 'rejected' | 'expired';

export interface Estimate {
	id: number;
	uuid: string;
	estimate_number: string;
	client_id: number;
	client_name?: string;
	date: string;
	valid_until: string;
	subtotal: number;
	tax_rate: number;
	tax_rate_id: number | null;
	tax_amount: number;
	total: number;
	notes: string;
	status: EstimateStatus;
	currency_code: string;
	converted_invoice_id: number | null;
	business_snapshot: string;
	client_snapshot: string;
	payer_snapshot: string;
	created_at: string;
	updated_at: string;
}

export interface EstimateLineItem {
	id: number;
	uuid: string;
	estimate_id: number;
	description: string;
	quantity: number;
	rate: number;
	amount: number;
	notes: string;
	sort_order: number;
	catalog_item_id: number | null;
	rate_tier_id: number | null;
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
	created_at: string;
	updated_at: string;
}

export interface RateTier {
	id: number;
	uuid: string;
	name: string;
	description: string;
	sort_order: number;
	created_at: string;
	updated_at: string;
}

export interface CatalogItemRate {
	id: number;
	catalog_item_id: number;
	rate_tier_id: number;
	rate: number;
}

export interface CatalogItemWithRates extends CatalogItem {
	rates: Record<number, number>;
}

export interface ColumnMapping {
	id: number;
	uuid: string;
	name: string;
	entity_type: string;
	mapping: string;
	tier_mapping: string;
	metadata_mapping: string;
	file_type: string;
	sheet_name: string;
	header_row: number;
	created_at: string;
	updated_at: string;
}

export interface AuditLogEntry {
	id: number;
	uuid: string;
	entity_type: string;
	entity_id: number | null;
	action: string;
	changes: string;
	context: string;
	batch_id: string | null;
	created_at: string;
}

export interface MonthlyRevenue {
	month: string; // YYYY-MM
	label: string; // e.g. "Jan 2024"
	revenue: number;
}

export interface ClientRevenueSummary {
	total_invoiced: number;
	total_paid: number;
	outstanding_balance: number;
	invoice_count: number;
	currency_code: string;
}

export interface AgingBucket {
	label: string;
	total: number;
	invoices: Invoice[];
}

export interface DashboardStats {
	total_revenue: number;
	outstanding_amount: number;
	overdue_count: number;
	total_clients: number;
	total_invoices: number;
	excluded_currency_count: number;
	recent_invoices: Invoice[];
	total_estimates: number;
	pending_estimates: number;
	recent_estimates: Estimate[];
}

export interface BusinessProfile {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	logo: string;
	metadata: string; // JSON string of Record<string, string>
	default_currency: string;
	created_at: string;
	updated_at: string;
}

export interface Payer {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string; // JSON string of Record<string, string>
	created_at: string;
	updated_at: string;
}

export interface PartySnapshot {
	name: string;
	email: string;
	phone: string;
	address: string;
	logo?: string;
	metadata: Record<string, string>;
}

export interface KeyValuePair {
	key: string;
	value: string;
}

export interface TaxRate {
	id: number;
	uuid: string;
	name: string;
	rate: number;
	is_default: number;
	created_at: string;
	updated_at: string;
}

export interface Payment {
	id: number;
	uuid: string;
	invoice_id: number;
	amount: number;
	payment_date: string;
	method: string;
	notes: string;
	created_at: string;
	updated_at: string;
}
