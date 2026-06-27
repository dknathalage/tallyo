<script lang="ts">
	import { adminTenants } from '$lib/stores/adminTenants.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column } from '$lib/components/datatable';
	import type { AdminTenantSummary } from '$lib/api/admin';

	// DataTable's own $effect issues the initial query() when it mounts, so no
	// onMount fetch here (a second call would just duplicate that request).

	// A 403 from the admin API means the signed-in user is not a platform admin.
	// Derived (not $state set from an $effect) so it clears the moment a later
	// query succeeds and resets the error — no stale access-denied banner.
	const forbidden = $derived(
		(adminTenants.error?.includes('403') ||
			adminTenants.error?.toLowerCase().includes('forbidden')) ??
			false
	);

	const columns: Column<AdminTenantSummary>[] = [
		{
			key: 'name',
			label: 'Name',
			sortable: true,
			filter: 'text'
		},
		{
			key: 'subscriptionStatus',
			label: 'Subscription',
			sortable: true,
			filter: 'enum',
			values: ['none', 'trialing', 'active', 'past_due', 'canceled'],
			cell: (r) => r.subscriptionStatus || 'none'
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: ['active', 'suspended'],
			cell: (r) => r.status
		},
		{
			key: 'userCount',
			label: 'Users',
			sortable: true,
			filter: 'number',
			cell: (r) => String(r.userCount)
		},
		{
			key: 'createdAt',
			label: 'Created',
			sortable: true,
			filter: 'date',
			cell: (r) => (r.createdAt ? new Date(r.createdAt).toLocaleDateString() : '—')
		}
	];
</script>

<div class="space-y-6">
	<div>
		<h1 class="mb-1 text-2xl font-semibold tracking-tight">Tenants</h1>
		<p class="text-sm text-gray-500">All platform tenants. Click a row to manage.</p>
	</div>

	{#if forbidden}
		<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
			Access denied — you must be a platform admin to view this page.
		</div>
	{:else if adminTenants.error}
		<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
			{adminTenants.error}
		</div>
	{:else}
		<!--
			DataTable renders enum columns as plain pills; we want coloured Badges for
			subscription status. The DataTable does not support custom cell renderers
			(cell() returns a string), so we use the standard table and accept the grey
			pill — a future iteration can extend DataTable with a snippet API.
		-->
		<DataTable
			title="Tenants"
			{columns}
			store={adminTenants}
			rowHref={(r) => `/admin/${r.id}`}
		/>
	{/if}
</div>
