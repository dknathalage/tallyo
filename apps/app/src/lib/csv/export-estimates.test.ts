import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('papaparse', () => ({
	default: { unparse: vi.fn().mockReturnValue('csv-content') }
}));

vi.mock('./download.js', () => ({
	downloadCsv: vi.fn()
}));

import Papa from 'papaparse';
import { downloadCsv } from './download.js';
import { exportEstimates } from './export-estimates.js';

const mockFetch = vi.fn();
const mockUnparse = vi.mocked(Papa.unparse);
const mockDownloadCsv = vi.mocked(downloadCsv);

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
});

describe('exportEstimates', () => {
	it('fetches from /api/export/estimates', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await exportEstimates();
		expect(mockFetch).toHaveBeenCalledWith('/api/export/estimates');
	});

	it('calls Papa.unparse with the fetched rows', async () => {
		const rows = [{ estimate_number: 'EST-001' }];
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue(rows) });
		await exportEstimates();
		expect(mockUnparse).toHaveBeenCalledWith(rows);
	});

	it('calls downloadCsv with estimates filename containing today date', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await exportEstimates();
		const today = new Date().toISOString().slice(0, 10);
		expect(mockDownloadCsv).toHaveBeenCalledWith(expect.any(String), `estimates-${today}.csv`);
	});

	it('passes the CSV string from Papa.unparse to downloadCsv', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		mockUnparse.mockReturnValue('estimate_number,date\nEST-001,2024-01-01');
		await exportEstimates();
		expect(mockDownloadCsv).toHaveBeenCalledWith('estimate_number,date\nEST-001,2024-01-01', expect.any(String));
	});

	it('handles empty rows array', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await expect(exportEstimates()).resolves.toBeUndefined();
	});
});
