import { apiGet, tenantPath } from '$lib/api/client';
import { onEntity } from '$lib/realtime/events';
import type { CatalogVersion, SupportItem, SupportItemPrice } from '$lib/api/types';

/**
 * Read-only store for the GLOBAL NDIS Support Catalogue. Tenants browse versions
 * + items; only platform admins may upload a new version. Refetches the version
 * list on the `catalog_version` SSE event (e.g. after an ingest).
 */
function createSupportCatalogStore() {
	let versions = $state<CatalogVersion[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let subscribed = false;

	async function loadVersions(): Promise<void> {
		loading = true;
		error = null;
		try {
			versions = (await apiGet<CatalogVersion[]>(tenantPath('support-catalog/versions'))) ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'failed to load catalogue versions';
		} finally {
			loading = false;
		}
	}

	async function loadItems(versionId: string): Promise<SupportItem[]> {
		if (versionId === '') {
			throw new Error('loadItems: versionId (uuid) is required');
		}
		return (await apiGet<SupportItem[]>(tenantPath(`support-catalog/versions/${versionId}/items`))) ??
			[];
	}

	async function loadPrices(itemId: string): Promise<SupportItemPrice[]> {
		if (itemId === '') {
			throw new Error('loadPrices: itemId (uuid) is required');
		}
		return (await apiGet<SupportItemPrice[]>(tenantPath(`support-catalog/items/${itemId}/prices`))) ?? [];
	}

	function ensureSubscribed(): void {
		if (subscribed) return;
		subscribed = true;
		onEntity('catalog_version', () => {
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
		loadPrices,
		ensureSubscribed
	};
}

export const supportCatalog = createSupportCatalogStore();
