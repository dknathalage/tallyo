import { apiGet, tenantPath } from '$lib/api/client';
import { onEntity } from '$lib/realtime/events';
import type { PriceListVersion, Item } from '$lib/api/types';

/**
 * Read-only store for the tenant-owned price list. Tenants browse versions +
 * items; only owner/admin may upload a new version. Refetches the version list
 * on the `price_list_version` SSE event (e.g. after an ingest).
 */
function createPriceListStore() {
	let versions = $state<PriceListVersion[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let subscribed = false;

	async function loadVersions(): Promise<void> {
		loading = true;
		error = null;
		try {
			versions = (await apiGet<PriceListVersion[]>(tenantPath('price-list/versions'))) ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'failed to load price-list versions';
		} finally {
			loading = false;
		}
	}

	async function loadItems(versionId: string): Promise<Item[]> {
		if (versionId === '') {
			throw new Error('loadItems: versionId (uuid) is required');
		}
		return (await apiGet<Item[]>(tenantPath(`price-list/versions/${versionId}/items`))) ??
			[];
	}

	function ensureSubscribed(): void {
		if (subscribed) return;
		subscribed = true;
		onEntity('price_list_version', () => {
			void loadVersions();
		});
	}

	return {
		get versions() {
			return versions;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		loadVersions,
		loadItems,
		ensureSubscribed
	};
}

export const priceList = createPriceListStore();
