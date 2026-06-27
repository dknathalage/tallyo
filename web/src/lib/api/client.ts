import { goto } from '$app/navigation';
import { page } from '$app/state';
import { getFirebaseAuth } from '$lib/firebase';
import type { ValidationDetail, EmailTenant } from './types';

/**
 * Resolve the current Firebase ID token for the Authorization header.
 * Returns null when no user is signed in (request goes out unauthenticated and
 * the server answers 401 → redirect). `forceRefresh` is used once after a 401 in
 * case the cached token went stale.
 */
async function authToken(forceRefresh = false): Promise<string | null> {
	if (typeof window === 'undefined') return null;
	try {
		const auth = await getFirebaseAuth();
		const u = auth.currentUser;
		if (!u) return null;
		return await u.getIdToken(forceRefresh);
	} catch {
		return null;
	}
}

/** Build request headers, attaching the Bearer token when a user is signed in. */
async function authHeaders(base: Record<string, string>, forceRefresh = false): Promise<Record<string, string>> {
	const token = await authToken(forceRefresh);
	if (token) {
		return { ...base, Authorization: `Bearer ${token}` };
	}
	return base;
}

/** Shape of an error body returned by the Go API. */
interface ApiErrorBody {
	error?: string;
	details?: ValidationDetail[];
	tenantRequired?: boolean;
	tenants?: EmailTenant[];
}

/**
 * Thrown for any non-2xx response (other than the 401 redirect path). It carries
 * the parsed error body so callers can read structured extras: `details` for a
 * 422 validation failure, and `tenantRequired`/`tenants` for the 409 multi-tenant
 * login disambiguation.
 */
export class ApiError extends Error {
	readonly status: number;
	readonly body: ApiErrorBody | null;
	constructor(status: number, message: string, body: ApiErrorBody | null) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.body = body;
	}

	/** Field-level validation failures from a 422 response (empty otherwise). */
	get details(): ValidationDetail[] {
		return this.body?.details ?? [];
	}

	/** True when a 409 login response asks the caller to pick a tenant. */
	get tenantRequired(): boolean {
		return this.status === 409 && this.body?.tenantRequired === true;
	}

	/** Candidate tenants for the 409 disambiguation flow (empty otherwise). */
	get tenants(): EmailTenant[] {
		return this.body?.tenants ?? [];
	}
}

/** Redirect to /login on 401, unless we're already on a public auth page. */
function handleUnauthorized(): void {
	if (typeof window === 'undefined') return;
	const path = page.url?.pathname ?? window.location.pathname;
	const publicPaths = ['/login', '/signup', '/accept-invite'];
	if (publicPaths.some((p) => path === p || path.startsWith(p + '/'))) return;
	void goto('/login');
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T | null> {
	if (typeof path !== 'string' || path.length === 0) {
		throw new Error('api request: path must be a non-empty string');
	}

	const serialized = body !== undefined ? JSON.stringify(body) : undefined;

	const doFetch = async (forceRefresh: boolean): Promise<Response> => {
		const init: RequestInit = {
			method,
			headers: await authHeaders({ 'Content-Type': 'application/json' }, forceRefresh)
		};
		if (serialized !== undefined) {
			init.body = serialized;
		}
		return fetch(path, init);
	};

	let res = await doFetch(false);

	// A 401 may just be a stale token: retry once with a force-refreshed token
	// before giving up and redirecting to /login.
	if (res.status === 401) {
		res = await doFetch(true);
		if (res.status === 401) {
			handleUnauthorized();
			return null;
		}
	}

	if (res.status === 204) {
		return null;
	}

	const text = await res.text();
	const data: unknown = text.length > 0 ? JSON.parse(text) : null;

	if (!res.ok) {
		// 402 = subscription required (a write hit the billing gate). Broadcast it so
		// the layout can show a paywall; a window event avoids a store import cycle.
		if (res.status === 402 && typeof window !== 'undefined') {
			window.dispatchEvent(new CustomEvent('billing:blocked'));
		}
		const errBody = (data as ApiErrorBody | null) ?? null;
		const errMsg = errBody?.error ?? `request failed (${res.status})`;
		throw new ApiError(res.status, errMsg, errBody);
	}

	return data as T;
}

// ---- Active tenant (URL-driven multi-tenancy) ----
// The [tenant] layout publishes the active tenant UUID here; tenant-scoped API
// calls build their path through tenantPath() so the prefix is computed PER
// REQUEST (never frozen at module-load time).
let _activeTenant: string | null = null;

/** Set (or clear) the active tenant UUID. Called by the [tenant] layout. */
export function setActiveTenant(uuid: string | null): void {
	_activeTenant = uuid;
}

/** The active tenant UUID, or null when none is set (e.g. on a public page). */
export function activeTenant(): string | null {
	return _activeTenant;
}

/**
 * Build a tenant-scoped API path: tenantPath('invoices') → /api/t/{uuid}/invoices.
 * Throws if no tenant is active — a tenant-scoped fetch before the layout set the
 * tenant is a programmer error (surfaces missing-prefix bugs immediately).
 */
export function tenantPath(resource: string): string {
	if (!_activeTenant) {
		throw new Error(`tenantPath(${resource}): no active tenant set`);
	}
	const r = resource.startsWith('/') ? resource.slice(1) : resource;
	return `/api/t/${_activeTenant}/${r}`;
}

export function apiGet<T>(path: string): Promise<T | null> {
	return request<T>('GET', path);
}

export function apiPost<T>(path: string, body?: unknown): Promise<T | null> {
	return request<T>('POST', path, body ?? {});
}

export function apiPut<T>(path: string, body?: unknown): Promise<T | null> {
	return request<T>('PUT', path, body ?? {});
}

export function apiPatch<T>(path: string, body?: unknown): Promise<T | null> {
	return request<T>('PATCH', path, body ?? {});
}

export function apiDelete<T>(path: string): Promise<T | null> {
	return request<T>('DELETE', path);
}

/**
 * Upload a multipart/form-data body (e.g. the price-list XLSX). Does not
 * set Content-Type (the browser adds the multipart boundary). Returns the parsed
 * JSON body, or throws ApiError on a non-2xx response.
 */
export async function apiUpload<T>(path: string, form: FormData): Promise<T | null> {
	if (typeof path !== 'string' || path.length === 0) {
		throw new Error('apiUpload: path must be a non-empty string');
	}
	if (!(form instanceof FormData)) {
		throw new Error('apiUpload: form must be a FormData');
	}
	const upload = (forceRefresh: boolean) =>
		authHeaders({}, forceRefresh).then((headers) =>
			fetch(path, { method: 'POST', headers, body: form })
		);
	let res = await upload(false);
	if (res.status === 401) {
		res = await upload(true);
		if (res.status === 401) {
			handleUnauthorized();
			return null;
		}
	}
	const text = await res.text();
	const data: unknown = text.length > 0 ? JSON.parse(text) : null;
	if (!res.ok) {
		const errBody = (data as ApiErrorBody | null) ?? null;
		const errMsg = errBody?.error ?? `upload failed (${res.status})`;
		throw new ApiError(res.status, errMsg, errBody);
	}
	return data as T;
}
