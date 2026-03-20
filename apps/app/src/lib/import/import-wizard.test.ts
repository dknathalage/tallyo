import { describe, it, expect, vi, beforeEach } from 'vitest';
import { applyMapping, type ColumnMappingConfig } from './map-columns.js';
import { diffCatalog } from './diff-catalog.js';

/**
 * Tests for the catalog import flow used by ImportWizardModal.
 * Validates that paginated API responses ({ data: [...] })
 * are handled correctly when extracting existing catalog items,
 * and that tier resolution reuses existing tiers without 409 errors.
 */

const mockFetch = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();
	globalThis.fetch = mockFetch;
});

function makeMappingConfig(overrides: Partial<ColumnMappingConfig> = {}): ColumnMappingConfig {
	return {
		fieldMap: {
			Name: 'name',
			SKU: 'sku',
			Rate: 'rate'
		},
		tierColumns: {},
		newTierColumns: [],
		metadataColumns: [],
		...overrides
	};
}

/**
 * Simulates the catalog fetch + diff logic from ImportWizardModal.handleModeSelected
 */
async function fetchAndDiffCatalog(
	rows: Record<string, string>[],
	mappingConfig: ColumnMappingConfig
) {
	const mapped = applyMapping(rows, mappingConfig);
	const existingRes = await fetch('/api/catalog?limit=200');
	const existingBody = await existingRes.json();
	const existingItems = existingBody.data ?? existingBody;
	const existing = existingItems.map(
		(item: { id: number; name: string; sku: string; rate: number; unit: string; category: string }) => ({
			id: item.id,
			name: item.name,
			sku: item.sku,
			rate: item.rate,
			unit: item.unit,
			category: item.category
		})
	);
	return diffCatalog(mapped, existing);
}

/**
 * Simulates tier resolution logic from ImportWizardModal.handleModeSelected.
 * Fetches existing tiers first and only creates ones that don't exist.
 */
async function resolveNewTiers(
	config: ColumnMappingConfig
): Promise<Record<string, number>> {
	const resolvedTierColumns = { ...config.tierColumns };
	const existingTiers: { id: number; name: string }[] = await fetch('/api/rate-tiers').then((r) => r.json());
	const tiersByName = new Map(existingTiers.map((t) => [t.name, t.id]));

	for (const colName of config.newTierColumns) {
		const existingId = tiersByName.get(colName);
		if (existingId) {
			resolvedTierColumns[colName] = existingId;
		} else {
			const res = await fetch('/api/rate-tiers', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ name: colName, description: `Auto-created from import column "${colName}"` })
			});
			if (!res.ok) {
				const errBody = await res.text();
				throw new Error(`Failed to create rate tier "${colName}": ${errBody}`);
			}
			const { id: tierId } = await res.json();
			resolvedTierColumns[colName] = tierId;
			tiersByName.set(colName, tierId);
		}
	}
	return resolvedTierColumns;
}

describe('ImportWizard catalog fetch handling', () => {
	const sampleRows = [
		{ Name: 'Widget', SKU: 'W-001', Rate: '10' },
		{ Name: 'Gadget', SKU: 'G-001', Rate: '20' }
	];
	const config = makeMappingConfig();

	it('handles paginated response with data property', async () => {
		mockFetch.mockResolvedValue({
			json: vi.fn().mockResolvedValue({
				data: [
					{ id: 1, name: 'Widget', sku: 'W-001', rate: 10, unit: 'ea', category: 'parts' }
				],
				total: 1,
				page: 1,
				limit: 200,
				totalPages: 1
			})
		});

		const result = await fetchAndDiffCatalog(sampleRows, config);
		expect(result).toBeDefined();
		expect(result.summary.new).toBe(1);
	});

	it('handles plain array response', async () => {
		mockFetch.mockResolvedValue({
			json: vi.fn().mockResolvedValue([
				{ id: 1, name: 'Widget', sku: 'W-001', rate: 10, unit: 'ea', category: 'parts' }
			])
		});

		const result = await fetchAndDiffCatalog(sampleRows, config);
		expect(result.summary.new).toBe(1);
	});

	it('handles empty paginated response', async () => {
		mockFetch.mockResolvedValue({
			json: vi.fn().mockResolvedValue({ data: [], total: 0, page: 1, limit: 200, totalPages: 0 })
		});

		const result = await fetchAndDiffCatalog(sampleRows, config);
		expect(result.summary.new).toBe(2);
		expect(result.summary.updated).toBe(0);
	});

	it('handles empty array response', async () => {
		mockFetch.mockResolvedValue({
			json: vi.fn().mockResolvedValue([])
		});

		const result = await fetchAndDiffCatalog(sampleRows, config);
		expect(result.summary.new).toBe(2);
		expect(result.summary.updated).toBe(0);
	});

	it('fetches with limit=200 query parameter', async () => {
		mockFetch.mockResolvedValue({
			json: vi.fn().mockResolvedValue({ data: [] })
		});

		await fetchAndDiffCatalog(sampleRows, config);
		expect(mockFetch).toHaveBeenCalledWith('/api/catalog?limit=200');
	});

	it('correctly maps existing items with extra fields', async () => {
		mockFetch.mockResolvedValue({
			json: vi.fn().mockResolvedValue({
				data: [
					{ id: 1, name: 'Widget', sku: 'W-001', rate: 10, unit: 'ea', category: 'parts', uuid: 'abc', metadata: '{}' }
				]
			})
		});

		const result = await fetchAndDiffCatalog(sampleRows, config);
		expect(result.summary.total).toBe(2);
	});
});

describe('ImportWizard tier resolution', () => {
	it('reuses existing tier without making a POST request', async () => {
		// GET /api/rate-tiers returns existing tiers
		mockFetch.mockResolvedValueOnce({
			json: vi.fn().mockResolvedValue([
				{ id: 7, name: 'Standard' },
				{ id: 13, name: 'Premium' }
			])
		});

		const config = makeMappingConfig({ newTierColumns: ['Premium'] });
		const result = await resolveNewTiers(config);

		expect(result['Premium']).toBe(13);
		// Only 1 call: GET to fetch existing tiers. No POST.
		expect(mockFetch).toHaveBeenCalledTimes(1);
		expect(mockFetch).toHaveBeenCalledWith('/api/rate-tiers');
	});

	it('creates a truly new tier via POST', async () => {
		// GET: no matching tier
		mockFetch.mockResolvedValueOnce({
			json: vi.fn().mockResolvedValue([{ id: 1, name: 'Standard' }])
		});
		// POST: create new tier
		mockFetch.mockResolvedValueOnce({
			ok: true,
			json: vi.fn().mockResolvedValue({ id: 42 })
		});

		const config = makeMappingConfig({ newTierColumns: ['Premium'] });
		const result = await resolveNewTiers(config);

		expect(result['Premium']).toBe(42);
		expect(mockFetch).toHaveBeenCalledTimes(2);
		expect(mockFetch).toHaveBeenNthCalledWith(2, '/api/rate-tiers', expect.objectContaining({
			method: 'POST',
			body: JSON.stringify({ name: 'Premium', description: 'Auto-created from import column "Premium"' })
		}));
	});

	it('handles mix of existing and new tiers', async () => {
		// GET: Premium exists, Gold does not
		mockFetch.mockResolvedValueOnce({
			json: vi.fn().mockResolvedValue([
				{ id: 13, name: 'Premium' }
			])
		});
		// POST: create Gold
		mockFetch.mockResolvedValueOnce({
			ok: true,
			json: vi.fn().mockResolvedValue({ id: 50 })
		});

		const config = makeMappingConfig({ newTierColumns: ['Premium', 'Gold'] });
		const result = await resolveNewTiers(config);

		expect(result['Premium']).toBe(13);
		expect(result['Gold']).toBe(50);
		// 1 GET + 1 POST (only for Gold)
		expect(mockFetch).toHaveBeenCalledTimes(2);
	});

	it('does not POST for any tier when all already exist', async () => {
		mockFetch.mockResolvedValueOnce({
			json: vi.fn().mockResolvedValue([
				{ id: 1, name: 'Tier A' },
				{ id: 2, name: 'Tier B' },
				{ id: 3, name: 'Tier C' }
			])
		});

		const config = makeMappingConfig({ newTierColumns: ['Tier A', 'Tier B', 'Tier C'] });
		const result = await resolveNewTiers(config);

		expect(result).toEqual({ 'Tier A': 1, 'Tier B': 2, 'Tier C': 3 });
		// Only the initial GET, zero POSTs
		expect(mockFetch).toHaveBeenCalledTimes(1);
	});

	it('throws on POST failure', async () => {
		mockFetch.mockResolvedValueOnce({
			json: vi.fn().mockResolvedValue([])
		});
		mockFetch.mockResolvedValueOnce({
			ok: false,
			text: vi.fn().mockResolvedValue('Internal Server Error')
		});

		const config = makeMappingConfig({ newTierColumns: ['Bad'] });
		await expect(resolveNewTiers(config)).rejects.toThrow('Failed to create rate tier "Bad": Internal Server Error');
	});

	it('preserves existing tierColumns', async () => {
		mockFetch.mockResolvedValueOnce({
			json: vi.fn().mockResolvedValue([])
		});
		mockFetch.mockResolvedValueOnce({
			ok: true,
			json: vi.fn().mockResolvedValue({ id: 10 })
		});

		const config = makeMappingConfig({
			tierColumns: { 'Existing': 5 },
			newTierColumns: ['New']
		});
		const result = await resolveNewTiers(config);
		expect(result['Existing']).toBe(5);
		expect(result['New']).toBe(10);
	});
});
