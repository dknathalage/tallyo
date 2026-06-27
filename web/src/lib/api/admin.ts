/**
 * Admin API client — platform super-admin endpoints at /api/admin/*.
 * These routes are gated by RequirePlatformAdmin on the server; a non-admin user
 * will receive a 403, which the ApiError machinery surfaces to callers.
 */
import { apiGet, apiPost, apiPatch, apiDelete } from './client';
import type { ListParams, ListResult } from './types';

// ---- Domain types ----

/** Full tenant row (GET /api/admin/tenants/:uuid → .tenant). */
export interface AdminTenant {
	id: string;
	name: string;
	status: string;
	createdAt: string;
	updatedAt: string;
	stripeCustomerId: string;
	stripeSubscriptionId: string;
	subscriptionStatus: string;
	trialEnd: string;
	currentPeriodEnd: string;
}

/** Tenant list row (GET /api/admin/tenants). Embeds AdminTenant + userCount. */
export interface AdminTenantSummary extends AdminTenant {
	userCount: number;
}

/** Audit trail record (GET /api/admin/tenants/:uuid → .audit[]). */
export interface AuditRecord {
	id: string;
	tenantId: string;
	userId: string;
	entityType: string;
	entityId: string;
	action: string;
	changes: string;
	context: string;
	batchId: string;
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

// ---- DataTable adapter ----

/**
 * Apply ListParams (sort/filter/page/limit) client-side on a flat array.
 * The /api/admin/tenants endpoint returns all tenants in one shot; we handle
 * pagination, sorting, and filtering here to satisfy the DataTable contract.
 */
export function applyListParams(
	all: AdminTenantSummary[],
	params: ListParams
): ListResult<AdminTenantSummary> {
	let rows = [...all];

	// ── Filtering ──
	const filters = params.filters ?? {};
	for (const [key, val] of Object.entries(filters)) {
		if (!val) continue;
		if (key === 'subscriptionStatus') {
			// enum filter: comma-joined values
			const opts = val.split(',').map((s) => s.trim()).filter(Boolean);
			if (opts.length > 0) {
				rows = rows.filter((r) => opts.includes(r.subscriptionStatus));
			}
		} else if (key === 'status') {
			const opts = val.split(',').map((s) => s.trim()).filter(Boolean);
			if (opts.length > 0) {
				rows = rows.filter((r) => opts.includes(r.status));
			}
		} else if (key === 'name') {
			const lower = val.toLowerCase();
			rows = rows.filter((r) => r.name.toLowerCase().includes(lower));
		} else if (key === 'userCount.min') {
			const min = Number(val);
			if (!isNaN(min)) rows = rows.filter((r) => r.userCount >= min);
		} else if (key === 'userCount.max') {
			const max = Number(val);
			if (!isNaN(max)) rows = rows.filter((r) => r.userCount <= max);
		}
	}

	// ── Sorting ──
	const sortKey = params.sort as keyof AdminTenantSummary | undefined;
	if (sortKey) {
		const dir = params.dir ?? 'asc';
		rows.sort((a, b) => {
			const av = a[sortKey];
			const bv = b[sortKey];
			if (av === bv) return 0;
			const cmp =
				typeof av === 'number' && typeof bv === 'number'
					? av - bv
					: String(av ?? '').localeCompare(String(bv ?? ''));
			return dir === 'asc' ? cmp : -cmp;
		});
	}

	// ── Pagination ──
	const total = rows.length;
	const page = params.page ?? 1;
	const limit = params.limit ?? 50;
	const start = (page - 1) * limit;
	const pageRows = rows.slice(start, start + limit);

	return { rows: pageRows, total };
}
