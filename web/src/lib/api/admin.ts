/**
 * Admin API client — platform super-admin endpoints at /api/admin/*.
 * These routes are gated by RequirePlatformAdmin on the server; a non-admin user
 * will receive a 403, which the ApiError machinery surfaces to callers.
 */
import { apiGet, apiPost, apiPatch, apiDelete } from './client';

// ---- Domain types ----

// AdminTenant/AdminTenantSummary and the client-side ListParams adapter live in
// the dependency-free ./listParams module (no `./client` import) so they are
// unit-testable without the firebase/SvelteKit module graph. Re-exported here so
// callers keep importing tenant types + the adapter from a single `./admin` path.
export type { AdminTenant, AdminTenantSummary } from './listParams';
export { applyListParams } from './listParams';

import type { AdminTenant, AdminTenantSummary } from './listParams';

/**
 * Audit trail record (GET /api/admin/tenants/:uuid → .audit[]).
 *
 * The Go audit.Record marks tenantId/userId/entityId/changes/context/batchId
 * `omitempty`, so those keys are absent (not "") when empty — they are optional
 * here to match the wire reality. id/entityType/action/createdAt are always sent.
 */
export interface AuditRecord {
	id: string;
	tenantId?: string;
	userId?: string;
	entityType: string;
	entityId?: string;
	action: string;
	changes?: string;
	context?: string;
	batchId?: string;
	createdAt: string;
}

/** Response shape for GET /api/admin/tenants/:uuid. */
export interface TenantDetailResponse {
	tenant: AdminTenant;
	audit: AuditRecord[];
}

/** Request body for PATCH /api/admin/tenants/:uuid/subscription. */
export interface SetSubscriptionRequest {
	status: string;
	trialEndsAt?: string;
}

// ---- Allowed subscription status values ----
export const SUBSCRIPTION_STATUSES = [
	'none',
	'trialing',
	'active',
	'past_due',
	'canceled'
] as const;
export type SubscriptionStatus = (typeof SUBSCRIPTION_STATUSES)[number];

// ---- API functions ----

/**
 * List all tenants. The server returns a flat array (not paginated), so we
 * implement client-side pagination for the DataTable contract via this adapter.
 */
export async function listTenants(): Promise<AdminTenantSummary[]> {
	const result = await apiGet<AdminTenantSummary[]>('/api/admin/tenants');
	return result ?? [];
}

/** Get a single tenant with its audit trail. */
export async function getTenant(uuid: string): Promise<TenantDetailResponse | null> {
	return apiGet<TenantDetailResponse>(`/api/admin/tenants/${uuid}`);
}

/** Override a tenant's subscription status. Returns null on success (204). */
export async function setSubscription(
	uuid: string,
	req: SetSubscriptionRequest
): Promise<null> {
	return apiPatch<null>(`/api/admin/tenants/${uuid}/subscription`, req);
}

/** Suspend a tenant. Returns null on success (204). */
export async function suspendTenant(uuid: string): Promise<null> {
	return apiPost<null>(`/api/admin/tenants/${uuid}/suspend`);
}

/** Unsuspend a tenant. Returns null on success (204). */
export async function unsuspendTenant(uuid: string): Promise<null> {
	return apiPost<null>(`/api/admin/tenants/${uuid}/unsuspend`);
}

/** Permanently delete a tenant. Returns null on success (204). */
export async function deleteTenant(uuid: string): Promise<null> {
	return apiDelete<null>(`/api/admin/tenants/${uuid}`);
}
