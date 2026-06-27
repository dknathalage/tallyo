/**
 * Dependency-free DataTable adapter for the admin tenant list.
 *
 * The /api/admin/tenants endpoint returns ALL tenants in one flat array (it is
 * not server-paginated), so the admin store applies sort/filter/paginate on the
 * client to satisfy the DataTable backing contract. This logic lives in its own
 * module — with no `./client` (and therefore no `$lib/firebase`) import — so it
 * is unit-testable under the plain `node` vitest environment.
 */
import type { ListParams, ListResult } from './types';

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

/**
 * Apply ListParams (sort/filter/page/limit) client-side on a flat array.
 * `total` is the post-filter, pre-pagination count; `rows` is the page slice.
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
		} else if (key === 'createdAt.from') {
			// DataTable emits date ranges as `<key>.from` / `<key>.to`. The values are
			// ISO-8601 date strings (yyyy-mm-dd), which compare correctly lexically
			// against the row's ISO-8601 createdAt timestamp. Inclusive lower bound.
			rows = rows.filter((r) => r.createdAt >= val);
		} else if (key === 'createdAt.to') {
			// Inclusive upper bound: a date-only `to` of 2024-01-15 must still match a
			// timestamp like 2024-01-15T09:00:00Z, so extend it to the end of the day.
			const upper = val.length === 10 ? val + 'T23:59:59.999Z' : val;
			rows = rows.filter((r) => r.createdAt <= upper);
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
