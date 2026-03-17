import { z } from 'zod';

export const CreateClientSchema = z.object({
	name: z.string().min(1, 'Name is required').max(255),
	email: z.string().email().max(255).optional().or(z.literal('')),
	phone: z.string().max(255).optional(),
	address: z.string().max(255).optional(),
	pricing_tier_id: z.number().int().positive().nullable().optional(),
	payer_id: z.number().int().positive().nullable().optional(),
	metadata: z.string().optional()
});

export const LineItemSchema = z.object({
	description: z.string().min(1, 'Description is required'),
	quantity: z.number().positive('Quantity must be positive'),
	rate: z.number().min(0, 'Rate must be non-negative'),
	amount: z.number().optional(),
	notes: z.string().optional(),
	sort_order: z.number().int().optional(),
	catalog_item_id: z.number().int().positive().nullable().optional(),
	rate_tier_id: z.number().int().positive().nullable().optional()
});

export const CreateInvoiceSchema = z.object({
	invoice_number: z.string().optional(),
	client_id: z.number().int().positive('Client ID must be a positive integer'),
	currency_code: z.string().length(3, 'Currency code must be 3 characters').optional(),
	date: z.string().optional(),
	due_date: z.string().optional(),
	notes: z.string().max(2000).optional(),
	tax_rate: z.number().min(0).optional(),
	tax_rate_id: z.number().int().positive().nullable().optional(),
	payment_terms: z.string().optional(),
	payer_id: z.number().int().positive().nullable().optional(),
	status: z.string().optional(),
	subtotal: z.number().optional(),
	tax_amount: z.number().optional(),
	total: z.number().optional(),
	business_snapshot: z.string().optional(),
	client_snapshot: z.string().optional(),
	payer_snapshot: z.string().optional()
});

export const CreateEstimateSchema = z.object({
	estimate_number: z.string().optional(),
	client_id: z.number().int().positive('Client ID must be a positive integer'),
	currency_code: z.string().length(3, 'Currency code must be 3 characters').optional(),
	date: z.string().optional(),
	valid_until: z.string().optional(),
	notes: z.string().max(2000).optional(),
	tax_rate: z.number().min(0).optional(),
	tax_rate_id: z.number().int().positive().nullable().optional(),
	payer_id: z.number().int().positive().nullable().optional(),
	status: z.string().optional(),
	subtotal: z.number().optional(),
	tax_amount: z.number().optional(),
	total: z.number().optional(),
	business_snapshot: z.string().optional(),
	client_snapshot: z.string().optional(),
	payer_snapshot: z.string().optional()
});

export const CreatePaymentSchema = z.object({
	invoice_id: z.number().int().positive('Invoice ID must be a positive integer'),
	amount: z.number().positive('Amount must be positive'),
	payment_date: z.string().min(1, 'Payment date is required'),
	method: z.string().optional(),
	notes: z.string().optional()
});

export const BulkDeleteSchema = z.object({
	ids: z.array(z.number().int().positive()).min(1, 'At least 1 ID required').max(1000, 'Maximum 1000 items')
});

export const SearchParamsSchema = z.object({
	search: z.string().max(255, 'Search query too long').optional(),
	page: z.number().int().positive().optional(),
	limit: z.number().int().min(1).max(200).optional()
});
