import { apiGet, apiPost, apiPut, apiDelete, tenantPath } from './client';
import type { ListParams, ListResult } from './types';

/** Encode ListParams into a query string (filter keys get an `f.` prefix). */
function toQueryString(p: ListParams): string {
	const u = new URLSearchParams();
	if (p.sort) u.set('sort', p.sort);
	if (p.dir) u.set('dir', p.dir);
	if (p.page) u.set('page', String(p.page));
	if (p.limit) u.set('limit', String(p.limit));
	for (const [k, v] of Object.entries(p.filters ?? {})) {
		if (v !== '') u.set('f.' + k, v);
	}
	const s = u.toString();
	return s ? '?' + s : '';
}

// NOTE: apiGet/apiPost/apiPut/apiDelete return Promise<T | null> (null on
// 401/204). This helper UNWRAPS: list() falls back to [], get/create/update throw
// on a null result (a 401 already redirected to /login, so reaching here with
// null is an error). Keeps a non-null Crud contract for callers.
export interface Crud<T, TInput> {
	list(): Promise<T[]>;
	query(params: ListParams): Promise<ListResult<T>>;
	get(id: number): Promise<T>;
	create(input: TInput): Promise<T>;
	update(id: number, input: TInput): Promise<T>;
	remove(id: number): Promise<void>;
}

function must<T>(v: T | null, what: string): T {
	if (v === null) throw new Error(`${what}: no data`);
	return v;
}

export function createCrud<T, TInput>(resource: string): Crud<T, TInput> {
	// Computed PER REQUEST (not at factory-build time) so it always reflects the
	// currently-active tenant — freezing it here would pin every store to the
	// first tenant that loaded.
	const base = () => tenantPath(resource);
	return {
		list: async () => (await apiGet<T[]>(base())) ?? [],
		query: async (params) =>
			(await apiGet<ListResult<T>>(`${base()}${toQueryString(params)}`)) ?? { rows: [], total: 0 },
		get: async (id) => must(await apiGet<T>(`${base()}/${id}`), `${resource} get`),
		create: async (input) => must(await apiPost<T>(base(), input), `${resource} create`),
		update: async (id, input) => must(await apiPut<T>(`${base()}/${id}`, input), `${resource} update`),
		remove: async (id) => {
			await apiDelete<void>(`${base()}/${id}`);
		}
	};
}
