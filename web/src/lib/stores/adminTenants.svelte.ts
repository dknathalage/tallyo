/**
 * Admin tenant store — satisfies the DataTable backing contract:
 *   { rows, total, loading, query(ListParams) }
 *
 * The /api/admin/tenants endpoint returns all tenants in one shot; we cache
 * them and apply sort/filter/paginate client-side so the DataTable column
 * controls work without additional round-trips.
 */
import { listTenants, applyListParams } from '$lib/api/admin';
import type { AdminTenantSummary } from '$lib/api/admin';
import type { ListParams } from '$lib/api/types';

function createAdminTenantsStore() {
	/** Full unfiltered list from the server. */
	let all = $state<AdminTenantSummary[]>([]);
	let rows = $state<AdminTenantSummary[]>([]);
	let total = $state(0);
	let loading = $state(false);
	let error = $state<string | null>(null);

	/** Fetch (or re-fetch) the full tenant list, then re-apply last params. */
	async function load(): Promise<void> {
		loading = true;
		error = null;
		try {
			all = await listTenants();
			rows = all;
			total = all.length;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	/** DataTable contract: sort/filter/paginate the cached list. */
	async function query(params: ListParams): Promise<void> {
		loading = true;
		error = null;
		try {
			// Reload from server if cache is empty (first call or after navigation).
			if (all.length === 0) {
				all = await listTenants();
			}
			const result = applyListParams(all, params);
			rows = result.rows;
			total = result.total;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	/**
	 * Evict the cache so the next query() re-fetches from the server. Also clears
	 * the currently displayed page so a stale list does not flash after a mutation
	 * (e.g. a deleted tenant lingering until the refetch resolves).
	 */
	function invalidate(): void {
		all = [];
		rows = [];
		total = 0;
	}

	return {
		get rows() {
			return rows;
		},
		get total() {
			return total;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		load,
		query,
		invalidate
	};
}

export const adminTenants = createAdminTenantsStore();
