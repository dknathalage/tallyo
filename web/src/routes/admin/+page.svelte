<script lang="ts">
	import { onMount } from 'svelte';
	import { adminTenants } from '$lib/stores/adminTenants.svelte';
	import { ApiError } from '$lib/api/client';
	import DataTable from '$lib/components/DataTable.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import type { Column } from '$lib/components/datatable';
	import type { AdminTenantSummary } from '$lib/api/admin';

	onMount(() => {
		void adminTenants.query({ page: 1, limit: 50 });
	});

	let forbidden = $state(false);

	// Watch for 403 errors from the store
	$effect(() => {
		if (adminTenants.error?.includes('403') || adminTenants.error?.toLowerCase().includes('forbidden')) {
			forbidden = true;
		}
	});

	// Subscription status → Badge tone mapping
	function subStatusTone(
		status: string
	): 'green' | 'blue' | 'amber' | 'red' | 'gray' {
		switch (status) {
			case 'active':
				return 'green';
			case 'trialing':
				return 'blue';
			case 'past_due':
				return 'amber';
			case 'canceled':
				return 'red';
			default:
				return 'gray';
		}
	}

	function tenantStatusTone(status: string): 'red' | 'gray' {
		return status === 'suspended' ? 'red' : 'gray';
	}

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
