import { describe, it, expect } from 'vitest';
import { applyListParams, type AdminTenantSummary } from './listParams';

// A small fixture of tenants with distinct names, statuses, user counts, and
// creation dates so each filter/sort branch is exercised independently.
function tenant(over: Partial<AdminTenantSummary>): AdminTenantSummary {
	return {
		id: over.id ?? 'id',
		name: over.name ?? 'Tenant',
		status: over.status ?? 'active',
		createdAt: over.createdAt ?? '2024-01-01T00:00:00Z',
		updatedAt: over.updatedAt ?? '2024-01-01T00:00:00Z',
		stripeCustomerId: '',
		stripeSubscriptionId: '',
		subscriptionStatus: over.subscriptionStatus ?? 'active',
		trialEnd: '',
		currentPeriodEnd: '',
		userCount: over.userCount ?? 1
	};
}

const fixture: AdminTenantSummary[] = [
	tenant({ id: 'a', name: 'Acme', subscriptionStatus: 'active', status: 'active', userCount: 5, createdAt: '2024-01-10T00:00:00Z' }),
	tenant({ id: 'b', name: 'Beta', subscriptionStatus: 'trialing', status: 'active', userCount: 2, createdAt: '2024-02-15T00:00:00Z' }),
	tenant({ id: 'c', name: 'Acme Corp', subscriptionStatus: 'canceled', status: 'suspended', userCount: 10, createdAt: '2024-03-20T00:00:00Z' }),
	tenant({ id: 'd', name: 'Delta', subscriptionStatus: 'past_due', status: 'active', userCount: 1, createdAt: '2024-04-25T00:00:00Z' })
];

describe('applyListParams — text filter', () => {
	it('matches name case-insensitively (contains)', () => {
		const res = applyListParams(fixture, { filters: { name: 'acme' } });
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['a', 'c']);
	});

	it('no match yields empty rows and zero total', () => {
		const res = applyListParams(fixture, { filters: { name: 'zzz' } });
		expect(res.total).toBe(0);
		expect(res.rows).toEqual([]);
	});
});

describe('applyListParams — enum filter', () => {
	it('subscriptionStatus single value', () => {
		const res = applyListParams(fixture, { filters: { subscriptionStatus: 'trialing' } });
		expect(res.total).toBe(1);
		expect(res.rows[0].id).toBe('b');
	});

	it('subscriptionStatus comma-joined values (set membership)', () => {
		const res = applyListParams(fixture, { filters: { subscriptionStatus: 'active,canceled' } });
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['a', 'c']);
	});

	it('tenant status enum', () => {
		const res = applyListParams(fixture, { filters: { status: 'suspended' } });
		expect(res.total).toBe(1);
		expect(res.rows[0].id).toBe('c');
	});
});

describe('applyListParams — number range filter', () => {
	it('userCount min only', () => {
		const res = applyListParams(fixture, { filters: { 'userCount.min': '5' } });
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['a', 'c']);
	});

	it('userCount max only', () => {
		const res = applyListParams(fixture, { filters: { 'userCount.max': '2' } });
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['b', 'd']);
	});

	it('userCount min and max (inclusive both ends)', () => {
		const res = applyListParams(fixture, {
			filters: { 'userCount.min': '2', 'userCount.max': '5' }
		});
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['a', 'b']);
	});
});

describe('applyListParams — date range filter', () => {
	it('createdAt from only (inclusive lower bound)', () => {
		const res = applyListParams(fixture, { filters: { 'createdAt.from': '2024-03-20' } });
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['c', 'd']);
	});

	it('createdAt to only (inclusive upper bound, end-of-day)', () => {
		const res = applyListParams(fixture, { filters: { 'createdAt.to': '2024-02-15' } });
		// 2024-02-15T00:00:00Z must be included even though to is date-only.
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['a', 'b']);
	});

	it('createdAt from and to (inclusive window)', () => {
		const res = applyListParams(fixture, {
			filters: { 'createdAt.from': '2024-02-01', 'createdAt.to': '2024-03-31' }
		});
		expect(res.total).toBe(2);
		expect(res.rows.map((r) => r.id).sort()).toEqual(['b', 'c']);
	});
});

describe('applyListParams — sorting', () => {
	it('sorts by name ascending', () => {
		const res = applyListParams(fixture, { sort: 'name', dir: 'asc' });
		expect(res.rows.map((r) => r.name)).toEqual(['Acme', 'Acme Corp', 'Beta', 'Delta']);
	});

	it('sorts by name descending', () => {
		const res = applyListParams(fixture, { sort: 'name', dir: 'desc' });
		expect(res.rows.map((r) => r.name)).toEqual(['Delta', 'Beta', 'Acme Corp', 'Acme']);
	});

	it('sorts numerically by userCount ascending', () => {
		const res = applyListParams(fixture, { sort: 'userCount', dir: 'asc' });
		expect(res.rows.map((r) => r.userCount)).toEqual([1, 2, 5, 10]);
	});
});

describe('applyListParams — pagination', () => {
	it('total reflects filtered count (pre-pagination); rows is the page slice', () => {
		const res = applyListParams(fixture, { sort: 'userCount', dir: 'asc', page: 1, limit: 2 });
		expect(res.total).toBe(4); // unpaginated total
		expect(res.rows.map((r) => r.userCount)).toEqual([1, 2]);
	});

	it('returns the second page', () => {
		const res = applyListParams(fixture, { sort: 'userCount', dir: 'asc', page: 2, limit: 2 });
		expect(res.total).toBe(4);
		expect(res.rows.map((r) => r.userCount)).toEqual([5, 10]);
	});

	it('combines filter then paginate; total is post-filter pre-pagination', () => {
		const res = applyListParams(fixture, {
			filters: { status: 'active' },
			sort: 'userCount',
			dir: 'asc',
			page: 1,
			limit: 2
		});
		expect(res.total).toBe(3); // a, b, d are active
		expect(res.rows.map((r) => r.userCount)).toEqual([1, 2]);
	});

	it('applies filter → sort → slice in order across a page boundary', () => {
		// Date window 2024-02-01..2024-04-30 keeps b, c, d (3 rows). Sorting those
		// by userCount asc → d(1), b(2), c(10). Page 2 @ limit 2 is the last slice,
		// holding only c — proving total is the post-filter count (3, not 4) and the
		// page is taken AFTER the filter+sort, not before.
		const res = applyListParams(fixture, {
			filters: { 'createdAt.from': '2024-02-01', 'createdAt.to': '2024-04-30' },
			sort: 'userCount',
			dir: 'asc',
			page: 2,
			limit: 2
		});
		expect(res.total).toBe(3);
		expect(res.rows.map((r) => r.id)).toEqual(['c']);
		expect(res.rows.map((r) => r.userCount)).toEqual([10]);
	});
});
