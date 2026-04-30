import { describe, it, expect } from 'vitest';
import {
	CLIENT_COLUMNS,
	CATALOG_COLUMNS,
	INVOICE_COLUMNS,
	ESTIMATE_COLUMNS,
	REQUIRED_CLIENT_FIELDS,
	REQUIRED_CATALOG_FIELDS,
	REQUIRED_INVOICE_FIELDS,
	REQUIRED_ESTIMATE_FIELDS
} from './columns.js';

describe('CLIENT_COLUMNS', () => {
	it('contains expected fields', () => {
		expect(CLIENT_COLUMNS).toContain('uuid');
		expect(CLIENT_COLUMNS).toContain('name');
		expect(CLIENT_COLUMNS).toContain('email');
		expect(CLIENT_COLUMNS).toContain('phone');
		expect(CLIENT_COLUMNS).toContain('address');
	});
});

describe('CATALOG_COLUMNS', () => {
	it('contains expected fields', () => {
		expect(CATALOG_COLUMNS).toContain('uuid');
		expect(CATALOG_COLUMNS).toContain('name');
		expect(CATALOG_COLUMNS).toContain('rate');
		expect(CATALOG_COLUMNS).toContain('unit');
		expect(CATALOG_COLUMNS).toContain('category');
		expect(CATALOG_COLUMNS).toContain('sku');
	});
});

describe('INVOICE_COLUMNS', () => {
	it('contains core invoice fields', () => {
		expect(INVOICE_COLUMNS).toContain('invoice_uuid');
		expect(INVOICE_COLUMNS).toContain('invoice_number');
		expect(INVOICE_COLUMNS).toContain('client_name');
		expect(INVOICE_COLUMNS).toContain('date');
		expect(INVOICE_COLUMNS).toContain('status');
	});

	it('contains line item fields', () => {
		expect(INVOICE_COLUMNS).toContain('line_description');
		expect(INVOICE_COLUMNS).toContain('line_quantity');
		expect(INVOICE_COLUMNS).toContain('line_rate');
		expect(INVOICE_COLUMNS).toContain('line_amount');
	});
});

describe('ESTIMATE_COLUMNS', () => {
	it('contains core estimate fields', () => {
		expect(ESTIMATE_COLUMNS).toContain('estimate_uuid');
		expect(ESTIMATE_COLUMNS).toContain('estimate_number');
		expect(ESTIMATE_COLUMNS).toContain('client_name');
		expect(ESTIMATE_COLUMNS).toContain('valid_until');
	});
});

describe('REQUIRED fields', () => {
	it('REQUIRED_CLIENT_FIELDS requires name', () => {
		expect(REQUIRED_CLIENT_FIELDS).toContain('name');
	});

	it('REQUIRED_CATALOG_FIELDS requires name', () => {
		expect(REQUIRED_CATALOG_FIELDS).toContain('name');
	});

	it('REQUIRED_INVOICE_FIELDS requires key fields', () => {
		expect(REQUIRED_INVOICE_FIELDS).toContain('invoice_number');
		expect(REQUIRED_INVOICE_FIELDS).toContain('client_name');
		expect(REQUIRED_INVOICE_FIELDS).toContain('date');
		expect(REQUIRED_INVOICE_FIELDS).toContain('line_description');
	});

	it('REQUIRED_ESTIMATE_FIELDS requires key fields', () => {
		expect(REQUIRED_ESTIMATE_FIELDS).toContain('estimate_number');
		expect(REQUIRED_ESTIMATE_FIELDS).toContain('client_name');
		expect(REQUIRED_ESTIMATE_FIELDS).toContain('date');
		expect(REQUIRED_ESTIMATE_FIELDS).toContain('line_description');
	});
});
