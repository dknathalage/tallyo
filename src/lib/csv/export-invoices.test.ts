import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('papaparse', () => ({
	default: { unparse: vi.fn().mockReturnValue('csv-content') }
}));

vi.mock('./download.js', () => ({
	downloadCsv: vi.fn()
}));

import Papa from 'papaparse';
import { downloadCsv } from './download.js';
import { exportInvoices } from './export-invoices.js';

const mockFetch = vi.fn();
const mockUnparse = vi.mocked(Papa.unparse);
const mockDownloadCsv = vi.mocked(downloadCsv);

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
});

describe('exportInvoices', () => {
	it('fetches from /api/export/invoices', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await exportInvoices();
		expect(mockFetch).toHaveBeenCalledWith('/api/export/invoices');
	});

	it('calls Papa.unparse with the fetched rows', async () => {
		const rows = [{ invoice_number: 'INV-001', client_name: 'Alice' }];
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue(rows) });
		await exportInvoices();
		expect(mockUnparse).toHaveBeenCalledWith(rows);
	});

	it('calls downloadCsv with the generated CSV', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		mockUnparse.mockReturnValue('fake,csv');
		await exportInvoices();
		expect(mockDownloadCsv).toHaveBeenCalledWith('fake,csv', expect.stringMatching(/^invoices-\d{4}-\d{2}-\d{2}\.csv$/));
	});

	it('includes the current date in the filename', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await exportInvoices();
		const today = new Date().toISOString().slice(0, 10);
		expect(mockDownloadCsv).toHaveBeenCalledWith(expect.any(String), `invoices-${today}.csv`);
	});

	it('handles empty rows array', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await expect(exportInvoices()).resolves.toBeUndefined();
		expect(mockDownloadCsv).toHaveBeenCalled();
	});
});
