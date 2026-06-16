import { apiGet, apiPost } from '$lib/api/client';
import type { User } from '$lib/api/types';

/**
 * Singleton session store. Holds the authenticated user so any page can gate UI
 * on role (owner/admin/member) or the orthogonal isPlatformAdmin flag without
 * refetching /api/auth/me. The layout calls refresh() on bootstrap.
 */
function createSessionStore() {
	let user = $state<User | null>(null);

	async function refresh(): Promise<User | null> {
		const me = await apiGet<User>('/api/auth/me');
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
	}

	return {
		get user() {
			return user;
		},
		get isManager(): boolean {
			return user !== null && (user.role === 'owner' || user.role === 'admin');
		},
		get isPlatformAdmin(): boolean {
			return user?.isPlatformAdmin === true;
		},
		refresh,
		set,
		logout
	};
}

export const session = createSessionStore();
