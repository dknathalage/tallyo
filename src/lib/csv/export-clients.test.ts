import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('papaparse', () => ({
	default: { unparse: vi.fn().mockReturnValue('csv-content') }
}));

vi.mock('./download.js', () => ({
	downloadCsv: vi.fn()
}));

import Papa from 'papaparse';
import { downloadCsv } from './download.js';
import { exportClients } from './export-clients.js';

const mockFetch = vi.fn();
const mockUnparse = vi.mocked(Papa.unparse);
const mockDownloadCsv = vi.mocked(downloadCsv);

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
});

describe('exportClients', () => {
	it('fetches from /api/clients', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await exportClients();
		expect(mockFetch).toHaveBeenCalledWith('/api/clients?limit=10000');
	});

	it('maps clients to only uuid, name, email, phone, address', async () => {
		const clients = [{
			uuid: 'abc-123', name: 'Alice', email: 'alice@example.com',
			phone: '555-1234', address: '123 Main St', extra_field: 'should-not-appear', pricing_tier_id: 2
		}];
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue(clients) });
		await exportClients();
		expect(mockUnparse).toHaveBeenCalledWith([{
			uuid: 'abc-123', name: 'Alice', email: 'alice@example.com',
			phone: '555-1234', address: '123 Main St'
		}]);
	});

	it('calls downloadCsv with clients filename containing today date', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await exportClients();
		const today = new Date().toISOString().slice(0, 10);
		expect(mockDownloadCsv).toHaveBeenCalledWith(expect.any(String), `clients-${today}.csv`);
	});

	it('handles empty clients array', async () => {
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue([]) });
		await expect(exportClients()).resolves.toBeUndefined();
		expect(mockUnparse).toHaveBeenCalledWith([]);
	});

	it('maps multiple clients correctly', async () => {
		const clients = [
			{ uuid: '1', name: 'Alice', email: 'a@a.com', phone: '111', address: 'Addr A' },
			{ uuid: '2', name: 'Bob', email: 'b@b.com', phone: '222', address: 'Addr B' }
		];
		mockFetch.mockResolvedValue({ json: vi.fn().mockResolvedValue(clients) });
		await exportClients();
		expect(mockUnparse).toHaveBeenCalledWith([
			{ uuid: '1', name: 'Alice', email: 'a@a.com', phone: '111', address: 'Addr A' },
			{ uuid: '2', name: 'Bob', email: 'b@b.com', phone: '222', address: 'Addr B' }
		]);
	});
});
