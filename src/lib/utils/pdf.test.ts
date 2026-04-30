import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock jspdf - must use class-like constructor with vi.fn().mockImplementation inside the factory
// because vi.mock is hoisted

const mockSave = vi.fn();
const mockText = vi.fn();
const mockSetFontSize = vi.fn();
const mockSetTextColor = vi.fn();
const mockSetDrawColor = vi.fn();
const mockSetLineWidth = vi.fn();
const mockLine = vi.fn();
const mockAddImage = vi.fn();
const mockSetFont = vi.fn();
const mockSplitTextToSize = vi.fn((...args: any[]) => [args[0]]);

// Use a factory that returns a constructed object
vi.mock('jspdf', () => {
	function MockJsPDF(this: any) {
		this.internal = {
			pageSize: { getWidth: () => 210, getHeight: () => 297 }
		};
		this.lastAutoTable = { finalY: 150 };
		this.save = (...args: any[]) => mockSave(...args);
		this.text = (...args: any[]) => mockText(...args);
		this.setFontSize = (...args: any[]) => mockSetFontSize(...args);
		this.setTextColor = (...args: any[]) => mockSetTextColor(...args);
		this.setDrawColor = (...args: any[]) => mockSetDrawColor(...args);
		this.setLineWidth = (...args: any[]) => mockSetLineWidth(...args);
		this.line = (...args: any[]) => mockLine(...args);
		this.addImage = (...args: any[]) => mockAddImage(...args);
		this.setFont = (...args: any[]) => mockSetFont(...args);
		this.splitTextToSize = (...args: any[]) => mockSplitTextToSize(...args);
	}
	return { jsPDF: MockJsPDF };
});

vi.mock('jspdf-autotable', () => ({
	default: vi.fn()
}));

// Mock i18n store
vi.mock('$lib/stores/i18n.svelte.js', () => ({
	i18n: {
		t: (key: string, values?: Record<string, string | number>) => {
			const map: Record<string, string> = {
				'pdf.invoice': 'INVOICE',
				'pdf.estimate': 'ESTIMATE',
				'pdf.serviceFor': 'SERVICE FOR',
				'pdf.billTo': 'BILL TO',
				'pdf.invoiceNumber': 'Invoice #:',
				'pdf.estimateNumber': 'Estimate #:',
				'pdf.date': 'Date:',
				'pdf.due': 'Due:',
				'pdf.validUntil': 'Valid Until:',
				'pdf.status': 'Status:',
				'pdf.description': 'Description',
				'pdf.quantity': 'Quantity',
				'pdf.rate': 'Rate',
				'pdf.amount': 'Amount',
				'pdf.subtotal': 'Subtotal:',
				'pdf.tax': `Tax (${values?.['rate'] ?? 0}%):`,
				'pdf.total': 'Total:',
				'pdf.notes': 'NOTES',
				'pdf.thankYou': 'Thank you for your business',
				'common.unknown': 'Unknown'
			};
			return map[key] ?? key;
		}
	}
}));

import { exportInvoicePdf, exportEstimatePdf } from './pdf.js';
import type { Invoice, LineItem, Estimate, EstimateLineItem } from '$lib/types/index.js';

const baseInvoice: Invoice = {
	id: 1,
	uuid: 'abc',
	invoice_number: 'INV-0001',
	client_id: 1,
	client_name: 'Test Client',
	date: '2025-01-15',
	due_date: '2025-02-15',
	payment_terms: 'net_30',
	subtotal: 100,
	tax_rate: 10,
	tax_rate_id: null,
	tax_amount: 10,
	total: 110,
	notes: 'Thank you',
	status: 'sent',
	currency_code: 'USD',
	business_snapshot: JSON.stringify({ name: 'My Business', address: '1 Main St\nCity', metadata: { ABN: '123' } }),
	client_snapshot: JSON.stringify({ name: 'Client Corp', email: 'client@test.com', phone: '555-0000', address: 'Client Ave', metadata: {} }),
	payer_snapshot: JSON.stringify({ name: 'Payer Inc', email: 'payer@test.com', phone: '555-9999', address: 'Payer St', metadata: { ref: 'P001' } }),
	created_at: '2025-01-15',
	updated_at: '2025-01-15'
};

const baseLineItems: LineItem[] = [
	{
		id: 1,
		uuid: 'li1',
		invoice_id: 1,
		description: 'Consulting',
		quantity: 2,
		rate: 50,
		amount: 100,
		notes: 'Per hour',
		sort_order: 0,
		catalog_item_id: null,
		rate_tier_id: null
	}
];

const baseEstimate: Estimate = {
	id: 1,
	uuid: 'est-1',
	estimate_number: 'EST-0001',
	client_id: 1,
	client_name: 'Estimate Client',
	date: '2025-03-01',
	valid_until: '2025-03-31',
	subtotal: 200,
	tax_rate: 5,
	tax_rate_id: null,
	tax_amount: 10,
	total: 210,
	notes: '',
	status: 'draft',
	currency_code: 'EUR',
	converted_invoice_id: null,
	business_snapshot: '{}',
	client_snapshot: '{}',
	payer_snapshot: '{}',
	created_at: '2025-03-01',
	updated_at: '2025-03-01'
};

const baseEstimateLineItems: EstimateLineItem[] = [
	{
		id: 1,
		uuid: 'eli1',
		estimate_id: 1,
		description: 'Design',
		quantity: 1,
		rate: 200,
		amount: 200,
		notes: '',
		sort_order: 0,
		catalog_item_id: null,
		rate_tier_id: null
	}
];

beforeEach(() => {
	vi.clearAllMocks();
	// Restore splitTextToSize implementation after clearAllMocks
	mockSplitTextToSize.mockImplementation((...args: any[]) => [args[0]]);
});

describe('exportInvoicePdf', () => {
	it('calls doc.save with correct filename', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockSave).toHaveBeenCalledWith('invoice-INV-0001.pdf');
	});

	it('renders text including invoice number', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockText).toHaveBeenCalledWith('INV-0001', expect.any(Number), expect.any(Number), expect.objectContaining({ align: 'right' }));
	});

	it('renders INVOICE title', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockText).toHaveBeenCalledWith('INVOICE', expect.any(Number), expect.any(Number), expect.objectContaining({ align: 'right' }));
	});

	it('renders business name', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockText).toHaveBeenCalledWith('My Business', expect.any(Number), expect.any(Number));
	});

	it('renders client name from snapshot', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockText).toHaveBeenCalledWith('Client Corp', expect.any(Number), expect.any(Number));
	});

	it('renders payer section when payer has a name', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockText).toHaveBeenCalledWith('Payer Inc', expect.any(Number), expect.any(Number));
	});

	it('renders notes when present', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		// splitTextToSize returns an array, so text is called with ['Thank you']
		expect(mockText).toHaveBeenCalledWith(['Thank you'], expect.any(Number), expect.any(Number));
	});

	it('works without notes', () => {
		const invoice = { ...baseInvoice, notes: '' };
		exportInvoicePdf(invoice, baseLineItems);
		expect(mockSave).toHaveBeenCalledWith('invoice-INV-0001.pdf');
	});

	it('works with no payer (empty payer snapshot)', () => {
		const invoice = { ...baseInvoice, payer_snapshot: '{}' };
		exportInvoicePdf(invoice, []);
		expect(mockSave).toHaveBeenCalledWith('invoice-INV-0001.pdf');
	});

	it('handles business logo in snapshot', () => {
		const invoice = {
			...baseInvoice,
			business_snapshot: JSON.stringify({ name: 'Biz', logo: 'data:image/png;base64,xyz', address: '', metadata: {} })
		};
		exportInvoicePdf(invoice, []);
		expect(mockAddImage).toHaveBeenCalled();
		expect(mockSave).toHaveBeenCalled();
	});

	it('handles broken logo gracefully (no throw)', () => {
		mockAddImage.mockImplementationOnce(() => { throw new Error('bad image'); });
		const invoice = {
			...baseInvoice,
			business_snapshot: JSON.stringify({ name: 'Biz', logo: 'bad-data', address: '', metadata: {} })
		};
		exportInvoicePdf(invoice, []);
		expect(mockSave).toHaveBeenCalled();
	});

	it('works with empty line items', () => {
		exportInvoicePdf(baseInvoice, []);
		expect(mockSave).toHaveBeenCalledWith('invoice-INV-0001.pdf');
	});

	it('handles line item with notes field', () => {
		exportInvoicePdf(baseInvoice, baseLineItems);
		expect(mockSave).toHaveBeenCalled();
	});

	it('uses USD as default when currency_code is empty string', () => {
		const invoice = { ...baseInvoice, currency_code: '' };
		exportInvoicePdf(invoice, baseLineItems);
		expect(mockSave).toHaveBeenCalled();
	});

	it('uses client_name fallback when client snapshot has no name', () => {
		const invoice = {
			...baseInvoice,
			client_snapshot: JSON.stringify({ email: 'x@y.com', phone: '', address: '', metadata: {} })
		};
		exportInvoicePdf(invoice, []);
		expect(mockText).toHaveBeenCalledWith('Test Client', expect.any(Number), expect.any(Number));
	});

	it('shows Unknown when no client name and no snapshot name', () => {
		const { client_name: _client_name, ...rest } = baseInvoice;
		void _client_name;
		const invoice = {
			...rest,
			client_snapshot: JSON.stringify({ email: '', phone: '', address: '', metadata: {} })
		};
		exportInvoicePdf(invoice, []);
		expect(mockText).toHaveBeenCalledWith('Unknown', expect.any(Number), expect.any(Number));
	});

	it('renders business address lines', () => {
		exportInvoicePdf(baseInvoice, []);
		// address '1 Main St\nCity' should produce two text calls
		expect(mockText).toHaveBeenCalledWith('1 Main St', expect.any(Number), expect.any(Number));
		expect(mockText).toHaveBeenCalledWith('City', expect.any(Number), expect.any(Number));
	});

	it('renders business metadata key-value pairs', () => {
		exportInvoicePdf(baseInvoice, []);
		expect(mockText).toHaveBeenCalledWith('ABN: 123', expect.any(Number), expect.any(Number));
	});
});

describe('exportEstimatePdf', () => {
	it('calls doc.save with correct estimate filename', () => {
		exportEstimatePdf(baseEstimate, baseEstimateLineItems);
		expect(mockSave).toHaveBeenCalledWith('estimate-EST-0001.pdf');
	});

	it('renders ESTIMATE title', () => {
		exportEstimatePdf(baseEstimate, baseEstimateLineItems);
		expect(mockText).toHaveBeenCalledWith('ESTIMATE', expect.any(Number), expect.any(Number), expect.objectContaining({ align: 'right' }));
	});

	it('renders estimate number', () => {
		exportEstimatePdf(baseEstimate, baseEstimateLineItems);
		expect(mockText).toHaveBeenCalledWith('EST-0001', expect.any(Number), expect.any(Number), expect.objectContaining({ align: 'right' }));
	});

	it('works with no notes', () => {
		exportEstimatePdf(baseEstimate, baseEstimateLineItems);
		expect(mockSave).toHaveBeenCalled();
	});

	it('works with empty line items', () => {
		exportEstimatePdf(baseEstimate, []);
		expect(mockSave).toHaveBeenCalledWith('estimate-EST-0001.pdf');
	});

	it('renders notes when present', () => {
		const estimate = { ...baseEstimate, notes: 'Valid for 30 days' };
		exportEstimatePdf(estimate, []);
		expect(mockText).toHaveBeenCalledWith(['Valid for 30 days'], expect.any(Number), expect.any(Number));
	});
});
