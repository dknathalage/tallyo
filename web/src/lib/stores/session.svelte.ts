import { apiGet, apiPost, tenantPath } from '$lib/api/client';
import type { User } from '$lib/api/types';

/**
 * One tenant membership for the signed-in account, from GET /api/auth/session.
 * `id` is the tenant's public UUID (the value used in the /{tenant} URL segment).
 */
export interface SessionTenant {
	id: string;
	tenantName: string;
	role: string;
}

/** The agnostic session payload: the account + every tenant it can access. */
export interface SessionInfo {
	email: string;
	tenants: SessionTenant[];
}

/**
 * Singleton session store. Splits two concerns:
 *  - the ACCOUNT-level session (email + tenant memberships) loaded once via the
 *    agnostic GET /api/auth/session — powers bootstrap and the tenant switcher;
 *  - the PER-TENANT user (with role / platform-admin flag) loaded via the
 *    tenant-scoped GET /api/auth/me — powers role gating within a tenant.
 * Any page can gate UI on role (owner/admin/member) or the orthogonal
 * isPlatformAdmin flag without refetching.
 */
function createSessionStore() {
	let user = $state<User | null>(null);
	let email = $state<string>('');
	let tenants = $state<SessionTenant[]>([]);

	/** Load the agnostic account session (email + tenants). */
	async function loadSession(): Promise<SessionInfo | null> {
		const info = await apiGet<SessionInfo>('/api/auth/session');
		email = info?.email ?? '';
		tenants = info?.tenants ?? [];
		return info;
	}

	/** Load the per-tenant user (requires an active tenant). */
	async function loadMe(): Promise<User | null> {
		const me = await apiGet<User>(tenantPath('auth/me'));
		user = me;
		return me;
	}

	function set(u: User | null): void {
		user = u;
	}

	async function logout(): Promise<void> {
		try {
			await apiPost('/api/auth/logout');
		} catch {
			// Ignore — clear local state regardless.
		}
		user = null;
		email = '';
		tenants = [];
	}

	return {
		get user() {
			return user;
		},
		get email(): string {
			return email;
		},
		get tenants(): SessionTenant[] {
			return tenants;
		},
		get isManager(): boolean {
			return user !== null && (user.role === 'owner' || user.role === 'admin');
		},
		get isPlatformAdmin(): boolean {
			return user?.isPlatformAdmin === true;
		},
		loadSession,
		loadMe,
		// Back-compat alias for the current layout bootstrap (which still calls
		// refresh()); the layout migration to loadSession/loadMe is a later task.
		refresh: loadMe,
		set,
		logout
	};
}

export const session = createSessionStore();
