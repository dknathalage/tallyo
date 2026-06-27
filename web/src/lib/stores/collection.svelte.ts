import { startPolling } from '$lib/realtime/poll';
import { createCrud, type Crud } from '$lib/api/crud';
import type { ListParams } from '$lib/api/types';

/**
 * Reusable rune-based collection store. Holds a reactive list, loads via the
 * CRUD helper, and refetches on a poll interval/focus for the given entity.
 *
 * Two read modes share one store:
 * - `load()` — fetch the whole collection into `items` (legacy/full-list pages).
 * - `query(params)` — server-side filter/sort/paginate into `rows`/`total` for
 *   the DataTable. After a query has run, the poll re-runs the LAST query
 *   (not a full reload) so the visible page stays consistent.
 */
export function createCollectionStore<T extends { id: string }, TInput>(
	resource: string,
	entity: string
) {
	if (typeof resource !== 'string' || resource.length === 0) {
		throw new Error('createCollectionStore: resource must be a non-empty string');
	}
	if (typeof entity !== 'string' || entity.length === 0) {
		throw new Error('createCollectionStore: entity must be a non-empty string');
	}

	const crud: Crud<T, TInput> = createCrud<T, TInput>(resource);
	let items = $state<T[]>([]);
	let rows = $state<T[]>([]);
	let total = $state(0);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let registered = false;
	let lastParams: ListParams | null = null;

	async function load(): Promise<void> {
		loading = true;
		error = null;
		try {
			items = await crud.list();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	/** Run a server-side list query; results land in `rows`/`total`. */
	async function query(params: ListParams): Promise<void> {
		lastParams = params;
		loading = true;
		error = null;
		try {
			const res = await crud.query(params);
			rows = res.rows;
			total = res.total;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	/** Start polling exactly once (browser only). */
	function ensureSubscribed(): void {
		if (registered) return;
		registered = true;
		startPolling(() => {
			// Re-run the active query if the page uses query(); else full reload.
			if (lastParams !== null) void query(lastParams);
			else void load();
		});
	}

	return {
		get items() {
			return items;
		},
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
		crud,
		load,
		query,
		ensureSubscribed
	};
}
