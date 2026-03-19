import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('papaparse', () => ({
	default: { unparse: vi.fn().mockReturnValue('csv-content') }
}));

vi.mock('./download.js', () => ({
	downloadCsv: vi.fn()
}));

import Papa from 'papaparse';
import { downloadCsv } from './download.js';
import { exportCatalog } from './export-catalog.js';

const mockFetch = vi.fn();
const mockUnparse = vi.mocked(Papa.unparse);
const mockDownloadCsv = vi.mocked(downloadCsv);

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
});

describe('exportCatalog', () => {
	it('fetches from /api/export/catalog', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ rows: [] }) });
		await exportCatalog();
		expect(mockFetch).toHaveBeenCalledWith('/api/export/catalog');
	});

	it('passes rows from { rows } response to Papa.unparse', async () => {
		const rows = [{ name: 'Item A', rate: 100 }, { name: 'Item B', rate: 200 }];
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ rows }) });
		await exportCatalog();
		expect(mockUnparse).toHaveBeenCalledWith(rows);
	});

	it('calls downloadCsv with catalog filename containing today date', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ rows: [] }) });
		await exportCatalog();
		const today = new Date().toISOString().slice(0, 10);
		expect(mockDownloadCsv).toHaveBeenCalledWith(expect.any(String), `catalog-${today}.csv`);
	});

	it('passes the CSV string to downloadCsv', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ rows: [] }) });
		mockUnparse.mockReturnValue('name,rate\nItem A,100');
		await exportCatalog();
		expect(mockDownloadCsv).toHaveBeenCalledWith('name,rate\nItem A,100', expect.any(String));
	});

	it('handles empty rows', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue({ rows: [] }) });
		await expect(exportCatalog()).resolves.toBeUndefined();
	});
});
