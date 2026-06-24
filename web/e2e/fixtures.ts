import type { APIRequestContext } from '@playwright/test';

// API seed helpers. Tests create the data they need through the real /api,
// not by clicking through setup screens — only the thing under test is driven
// via the UI.

export const OWNER = {
	businessName: 'E2E Test Co',
	name: 'E2E Owner',
	email: 'e2e-owner@example.com',
	password: 'password1234' // >= minPasswordLen (8)
};

// signupOwner provisions the first tenant + owner and logs the request context
// in (cookie stored on `api`). Returns the tenant uuid.
//
// The signup response serializes the owner but currently leaves `tenantId` empty
// (the signup path skips the tenant-uuid backfill that the other user lookups
// do). Rather than depend on that field, we resolve the tenant uuid from the
// now-authenticated session via GET /api/auth/session, which lists the tenants
// for the logged-in email (each `id` is the tenant uuid).
export async function signupOwner(api: APIRequestContext): Promise<string> {
	const res = await api.post('/api/signup', { data: OWNER });
	if (!res.ok()) throw new Error(`signup failed: ${res.status()} ${await res.text()}`);

	const owner = await res.json();
	if (owner.tenantId) return owner.tenantId as string;

	const sess = await api.get('/api/auth/session');
	if (!sess.ok()) throw new Error(`auth/session failed: ${sess.status()} ${await sess.text()}`);
	const body = (await sess.json()) as { tenants?: Array<{ id: string }> };
	const tenant = body.tenants?.[0]?.id;
	if (!tenant) throw new Error(`no tenant for signed-up owner: ${JSON.stringify(body)}`);
	return tenant;
}

// createClient seeds one client under a tenant. Returns its uuid (`id`).
export async function createClient(
	api: APIRequestContext,
	tenant: string,
	name: string
): Promise<string> {
	const res = await api.post(`/api/t/${tenant}/clients`, { data: { name } });
	if (!res.ok()) throw new Error(`createClient failed: ${res.status()} ${await res.text()}`);
	const c = await res.json();
	if (!c.id) throw new Error(`client response missing id: ${JSON.stringify(c)}`);
	return c.id as string;
}
