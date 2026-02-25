export interface Client {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	pricing_tier_id: number | null;
	pricing_tier_name?: string;
	created_at: string;
	updated_at: string;
}

export interface Invoice {
	id: number;
	uuid: string;
	invoice_number: string;
	client_id: number;
	client_name?: string;
	date: string;
	due_date: string;
	subtotal: number;
	tax_rate: number;
	tax_amount: number;
	total: number;
	notes: string;
	status: 'draft' | 'sent' | 'paid' | 'overdue';
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

export interface DashboardStats {
	total_revenue: number;
	outstanding_amount: number;
	overdue_count: number;
	total_clients: number;
	total_invoices: number;
	recent_invoices: Invoice[];
}
