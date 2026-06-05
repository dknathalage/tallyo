import { onEntity } from '$lib/realtime/events';
import { createCrud, type Crud } from '$lib/api/crud';

/**
 * Reusable rune-based collection store. Holds a reactive list, loads via the
 * CRUD helper, and refetches on SSE invalidation for the given entity.
 */
export function createCollectionStore<T extends { id: number }, TInput>(
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
	let loading = $state(false);
	let error = $state<string | null>(null);
	let registered = false;

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

	/** Subscribe to SSE invalidations exactly once (browser only). */
	function ensureSubscribed(): void {
		if (registered) return;
		registered = true;
		onEntity(entity, () => {
			void load();
		});
	}

	return {
		get items() {
			return items;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		crud,
		load,
		ensureSubscribed
	};
}
