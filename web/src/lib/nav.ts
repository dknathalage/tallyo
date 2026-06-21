import { page } from '$app/state';
/** Prefix an app path with the active tenant from the route: t('/invoices') → /{tenant}/invoices */
export function t(path: string): string {
	const tenant = page.params.tenant;
	if (!tenant) throw new Error('t(): no tenant in current route');
	return `/${tenant}${path.startsWith('/') ? path : '/' + path}`;
}
