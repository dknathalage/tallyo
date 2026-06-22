import type { LayoutLoad } from './$types';
import { setActiveTenant } from '$lib/api/client';

// Publish the active tenant BEFORE any child component mounts / onMount runs, so
// tenant-scoped fetches (tenantPath) never race the layout effect. Universal load
// re-runs whenever params.tenant changes, covering tenant switching.
export const load: LayoutLoad = ({ params }) => {
	setActiveTenant(params.tenant ?? null);
	return {};
};
