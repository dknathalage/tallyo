import { describe, it, expect } from 'vitest';
import {
	CreateClientSchema,
	LineItemSchema,
	CreateInvoiceSchema,
	CreateEstimateSchema,
	CreatePaymentSchema,
	BulkDeleteSchema,
	SearchParamsSchema
} from './schemas.js';

describe('CreateClientSchema', () => {
	it('accepts minimal valid input', () => {
		const result = CreateClientSchema.safeParse({ name: 'Alice' });
		expect(result.success).toBe(true);
	});

	it('accepts full valid input', () => {
		const result = CreateClientSchema.safeParse({
			name: 'Alice',
			email: 'alice@example.com',
			phone: '555-0100',
			address: '123 Main St',
			pricing_tier_id: 1,
			payer_id: 2,
			metadata: '{"ABN":"123"}'
		});
		expect(result.success).toBe(true);
	});

	it('accepts empty string email', () => {
		const result = CreateClientSchema.safeParse({ name: 'Alice', email: '' });
		expect(result.success).toBe(true);
	});

	it('rejects missing name', () => {
		const result = CreateClientSchema.safeParse({});
		expect(result.success).toBe(false);
	});

	it('rejects empty name', () => {
		const result = CreateClientSchema.safeParse({ name: '' });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Name is required');
	});

	it('rejects invalid email', () => {
		const result = CreateClientSchema.safeParse({ name: 'Alice', email: 'not-an-email' });
		expect(result.success).toBe(false);
	});

	it('rejects non-positive pricing_tier_id', () => {
		const result = CreateClientSchema.safeParse({ name: 'Alice', pricing_tier_id: 0 });
		expect(result.success).toBe(false);
	});

	it('accepts null pricing_tier_id', () => {
		const result = CreateClientSchema.safeParse({ name: 'Alice', pricing_tier_id: null });
		expect(result.success).toBe(true);
	});

	it('accepts null payer_id', () => {
		const result = CreateClientSchema.safeParse({ name: 'Alice', payer_id: null });
		expect(result.success).toBe(true);
	});

	it('rejects name over 255 chars', () => {
		const result = CreateClientSchema.safeParse({ name: 'a'.repeat(256) });
		expect(result.success).toBe(false);
	});
});

describe('LineItemSchema', () => {
	it('accepts valid line item', () => {
		const result = LineItemSchema.safeParse({
			description: 'Service A',
			quantity: 2,
			rate: 100
		});
		expect(result.success).toBe(true);
	});

	it('accepts full line item', () => {
		const result = LineItemSchema.safeParse({
			description: 'Service A',
			quantity: 2,
			rate: 100,
			amount: 200,
			notes: 'some note',
			sort_order: 0,
			catalog_item_id: 5,
			rate_tier_id: 3
		});
		expect(result.success).toBe(true);
	});

	it('rejects missing description', () => {
		const result = LineItemSchema.safeParse({ quantity: 1, rate: 100 });
		expect(result.success).toBe(false);
	});

	it('rejects empty description', () => {
		const result = LineItemSchema.safeParse({ description: '', quantity: 1, rate: 100 });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Description is required');
	});

	it('rejects zero quantity', () => {
		const result = LineItemSchema.safeParse({ description: 'X', quantity: 0, rate: 100 });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Quantity must be positive');
	});

	it('rejects negative quantity', () => {
		const result = LineItemSchema.safeParse({ description: 'X', quantity: -1, rate: 100 });
		expect(result.success).toBe(false);
	});

	it('rejects negative rate', () => {
		const result = LineItemSchema.safeParse({ description: 'X', quantity: 1, rate: -1 });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Rate must be non-negative');
	});

	it('accepts zero rate', () => {
		const result = LineItemSchema.safeParse({ description: 'X', quantity: 1, rate: 0 });
		expect(result.success).toBe(true);
	});

	it('accepts null catalog_item_id', () => {
		const result = LineItemSchema.safeParse({ description: 'X', quantity: 1, rate: 0, catalog_item_id: null });
		expect(result.success).toBe(true);
	});
});

describe('CreateInvoiceSchema', () => {
	it('accepts minimal valid input', () => {
		const result = CreateInvoiceSchema.safeParse({ client_id: 1 });
		expect(result.success).toBe(true);
	});

	it('accepts full valid input', () => {
		const result = CreateInvoiceSchema.safeParse({
			client_id: 1,
			currency_code: 'USD',
			date: '2025-01-01',
			due_date: '2025-02-01',
			notes: 'Thanks',
			tax_rate: 10,
			tax_rate_id: 1,
			payment_terms: 'Net 30',
			payer_id: 2,
			status: 'draft',
			subtotal: 100,
			tax_amount: 10,
			total: 110,
			business_snapshot: '{}',
			client_snapshot: '{}',
			payer_snapshot: '{}'
		});
		expect(result.success).toBe(true);
	});

	it('rejects missing client_id', () => {
		const result = CreateInvoiceSchema.safeParse({});
		expect(result.success).toBe(false);
	});

	it('rejects zero client_id', () => {
		const result = CreateInvoiceSchema.safeParse({ client_id: 0 });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Client ID must be a positive integer');
	});

	it('rejects negative client_id', () => {
		const result = CreateInvoiceSchema.safeParse({ client_id: -5 });
		expect(result.success).toBe(false);
	});

	it('rejects invalid currency_code length', () => {
		const result = CreateInvoiceSchema.safeParse({ client_id: 1, currency_code: 'US' });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Currency code must be 3 characters');
	});

	it('rejects notes over 2000 chars', () => {
		const result = CreateInvoiceSchema.safeParse({ client_id: 1, notes: 'a'.repeat(2001) });
		expect(result.success).toBe(false);
	});

	it('accepts null payer_id', () => {
		const result = CreateInvoiceSchema.safeParse({ client_id: 1, payer_id: null });
		expect(result.success).toBe(true);
	});
});

describe('CreateEstimateSchema', () => {
	it('accepts minimal valid input', () => {
		const result = CreateEstimateSchema.safeParse({ client_id: 1 });
		expect(result.success).toBe(true);
	});

	it('accepts full valid input', () => {
		const result = CreateEstimateSchema.safeParse({
			client_id: 1,
			currency_code: 'EUR',
			date: '2025-01-01',
			valid_until: '2025-02-01',
			notes: 'Valid for 30 days',
			tax_rate: 5,
			tax_rate_id: 2,
			payer_id: 3,
			status: 'draft',
			subtotal: 200,
			tax_amount: 10,
			total: 210,
			business_snapshot: '{}',
			client_snapshot: '{}',
			payer_snapshot: '{}'
		});
		expect(result.success).toBe(true);
	});

	it('rejects missing client_id', () => {
		const result = CreateEstimateSchema.safeParse({});
		expect(result.success).toBe(false);
	});

	it('rejects zero client_id', () => {
		const result = CreateEstimateSchema.safeParse({ client_id: 0 });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Client ID must be a positive integer');
	});

	it('rejects currency_code with wrong length', () => {
		const result = CreateEstimateSchema.safeParse({ client_id: 1, currency_code: 'EURO' });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Currency code must be 3 characters');
	});

	it('accepts null tax_rate_id', () => {
		const result = CreateEstimateSchema.safeParse({ client_id: 1, tax_rate_id: null });
		expect(result.success).toBe(true);
	});
});

describe('CreatePaymentSchema', () => {
	it('accepts valid input', () => {
		const result = CreatePaymentSchema.safeParse({
			invoice_id: 1,
			amount: 100,
			payment_date: '2025-01-15'
		});
		expect(result.success).toBe(true);
	});

	it('accepts optional fields', () => {
		const result = CreatePaymentSchema.safeParse({
			invoice_id: 1,
			amount: 100,
			payment_date: '2025-01-15',
			method: 'bank_transfer',
			notes: 'Full payment'
		});
		expect(result.success).toBe(true);
	});

	it('rejects missing invoice_id', () => {
		const result = CreatePaymentSchema.safeParse({ amount: 100, payment_date: '2025-01-15' });
		expect(result.success).toBe(false);
	});

	it('rejects zero invoice_id', () => {
		const result = CreatePaymentSchema.safeParse({ invoice_id: 0, amount: 100, payment_date: '2025-01-15' });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Invoice ID must be a positive integer');
	});

	it('rejects zero amount', () => {
		const result = CreatePaymentSchema.safeParse({ invoice_id: 1, amount: 0, payment_date: '2025-01-15' });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Amount must be positive');
	});

	it('rejects negative amount', () => {
		const result = CreatePaymentSchema.safeParse({ invoice_id: 1, amount: -50, payment_date: '2025-01-15' });
		expect(result.success).toBe(false);
	});

	it('rejects empty payment_date', () => {
		const result = CreatePaymentSchema.safeParse({ invoice_id: 1, amount: 100, payment_date: '' });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Payment date is required');
	});

	it('rejects missing payment_date', () => {
		const result = CreatePaymentSchema.safeParse({ invoice_id: 1, amount: 100 });
		expect(result.success).toBe(false);
	});
});

describe('BulkDeleteSchema', () => {
	it('accepts array of valid ids', () => {
		const result = BulkDeleteSchema.safeParse({ ids: [1, 2, 3] });
		expect(result.success).toBe(true);
	});

	it('accepts single id', () => {
		const result = BulkDeleteSchema.safeParse({ ids: [1] });
		expect(result.success).toBe(true);
	});

	it('rejects empty array', () => {
		const result = BulkDeleteSchema.safeParse({ ids: [] });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('At least 1 ID required');
	});

	it('rejects array with 1001 items', () => {
		const ids = Array.from({ length: 1001 }, (_, i) => i + 1);
		const result = BulkDeleteSchema.safeParse({ ids });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Maximum 1000 items');
	});

	it('accepts array with exactly 1000 items', () => {
		const ids = Array.from({ length: 1000 }, (_, i) => i + 1);
		const result = BulkDeleteSchema.safeParse({ ids });
		expect(result.success).toBe(true);
	});

	it('rejects non-positive ids', () => {
		const result = BulkDeleteSchema.safeParse({ ids: [0, 1, 2] });
		expect(result.success).toBe(false);
	});

	it('rejects negative ids', () => {
		const result = BulkDeleteSchema.safeParse({ ids: [-1] });
		expect(result.success).toBe(false);
	});

	it('rejects missing ids', () => {
		const result = BulkDeleteSchema.safeParse({});
		expect(result.success).toBe(false);
	});
});

describe('SearchParamsSchema', () => {
	it('accepts empty object', () => {
		const result = SearchParamsSchema.safeParse({});
		expect(result.success).toBe(true);
	});

	it('accepts all fields', () => {
		const result = SearchParamsSchema.safeParse({ search: 'test', page: 1, limit: 50 });
		expect(result.success).toBe(true);
	});

	it('rejects search over 255 chars', () => {
		const result = SearchParamsSchema.safeParse({ search: 'a'.repeat(256) });
		expect(result.success).toBe(false);
		expect(result.error?.issues[0]?.message).toBe('Search query too long');
	});

	it('rejects page 0', () => {
		const result = SearchParamsSchema.safeParse({ page: 0 });
		expect(result.success).toBe(false);
	});

	it('rejects limit 0', () => {
		const result = SearchParamsSchema.safeParse({ limit: 0 });
		expect(result.success).toBe(false);
	});

	it('rejects limit over 200', () => {
		const result = SearchParamsSchema.safeParse({ limit: 201 });
		expect(result.success).toBe(false);
	});

	it('accepts limit 200', () => {
		const result = SearchParamsSchema.safeParse({ limit: 200 });
		expect(result.success).toBe(true);
	});

	it('accepts page 1', () => {
		const result = SearchParamsSchema.safeParse({ page: 1 });
		expect(result.success).toBe(true);
	});
});
