export interface Client {
	id: number;
	name: string;
	email: string;
	phone: string;
	address: string;
	created_at: string;
	updated_at: string;
}

export interface Invoice {
	id: number;
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
	invoice_id: number;
	description: string;
	quantity: number;
	rate: number;
	amount: number;
	sort_order: number;
}

export interface DashboardStats {
	total_revenue: number;
	outstanding_amount: number;
	overdue_count: number;
	total_clients: number;
	total_invoices: number;
	recent_invoices: Invoice[];
}
