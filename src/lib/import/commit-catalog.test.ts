import { describe, it, expect, vi, beforeEach } from 'vitest';
import { commitCatalogImport } from './commit-catalog.js';
import type { DiffResult } from './diff-catalog.js';

const mockFetch = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
});

function makeDiffResult(overrides: Partial<DiffResult> = {}): DiffResult {
	return {
		newItems: [], updatedItems: [], unchangedCount: 0, errorItems: [],
		summary: { total: 0, new: 0, updated: 0, unchanged: 0, errors: 0 }, ...overrides
	};
}

describe('commitCatalogImport', () => {
	it('sends POST request to /api/import/catalog', async () => {
		mockFetch.mockResolvedValue({
			ok: true,
			json: vi.fn().mockResolvedValue({ inserted: 2, updated: 1, batchId: 'batch-1' })
		});
		await commitCatalogImport(makeDiffResult(), { updateExisting: false });
		expect(mockFetch).toHaveBeenCalledWith('/api/import/catalog', expect.objectContaining({ method: 'POST' }));
	});

	it('sends Content-Type application/json header', async () => {
		mockFetch.mockResolvedValue({
			ok: true,
			json: vi.fn().mockResolvedValue({ inserted: 0, updated: 0, batchId: 'b1' })
		});
		await commitCatalogImport(makeDiffResult(), { updateExisting: false });
		expect(mockFetch).toHaveBeenCalledWith(
			expect.any(String),
			expect.objectContaining({ headers: { 'Content-Type': 'application/json' } })
		);
	});

	it('sends diff and options in the request body as JSON', async () => {
		mockFetch.mockResolvedValue({
			ok: true,
			json: vi.fn().mockResolvedValue({ inserted: 1, updated: 0, batchId: 'b1' })
		});
		const diff = makeDiffResult({ summary: { total: 1, new: 1, updated: 0, unchanged: 0, errors: 0 } });
		const options = { updateExisting: true };
		await commitCatalogImport(diff, options);
		expect(mockFetch).toHaveBeenCalledWith(
			expect.any(String),
			expect.objectContaining({ body: JSON.stringify({ diff, options }) })
		);
	});

	it('returns the response JSON on success', async () => {
		const responseData = { inserted: 5, updated: 2, batchId: 'batch-xyz' };
		mockFetch.mockResolvedValue({ ok: true, json: vi.fn().mockResolvedValue(responseData) });
		const result = await commitCatalogImport(makeDiffResult(), { updateExisting: false });
		expect(result).toEqual(responseData);
	});

	it('throws an error when response is not ok', async () => {
		mockFetch.mockResolvedValue({ ok: false, text: vi.fn().mockResolvedValue('Internal Server Error') });
		await expect(
			commitCatalogImport(makeDiffResult(), { updateExisting: false })
		).rejects.toThrow('Catalog import failed: Internal Server Error');
	});

	it('includes updateExisting: true in body when specified', async () => {
		mockFetch.mockResolvedValue({ ok: true, json: vi.fn().mockResolvedValue({ inserted: 0, updated: 3, batchId: 'b2' }) });
		await commitCatalogImport(makeDiffResult(), { updateExisting: true });
		const callArgs = mockFetch.mock.calls[0]?.[1];
		if (!callArgs) throw new Error('expected fetch to be called');
		const body = JSON.parse(callArgs.body);
		expect(body.options.updateExisting).toBe(true);
	});

	it('includes updateExisting: false in body when specified', async () => {
		mockFetch.mockResolvedValue({ ok: true, json: vi.fn().mockResolvedValue({ inserted: 0, updated: 0, batchId: 'b3' }) });
		await commitCatalogImport(makeDiffResult(), { updateExisting: false });
		const callArgs = mockFetch.mock.calls[0]?.[1];
		if (!callArgs) throw new Error('expected fetch to be called');
		const body = JSON.parse(callArgs.body);
		expect(body.options.updateExisting).toBe(false);
	});
});
