<script lang="ts">
	import { getClients } from '$lib/db/queries/clients';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Button from '$lib/components/shared/Button.svelte';

	let search = $state('');

	let clients = $derived(getClients(search || undefined));
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Clients</h1>
		<a href="/clients/new">
			<Button>New Client</Button>
		</a>
	</div>

	<!-- Search -->
	<div class="max-w-sm">
		<SearchInput bind:value={search} placeholder="Search clients..." />
	</div>

	<!-- Client list -->
	{#if clients.length === 0}
		{#if search}
			<EmptyState title="No results" message="No clients match your search. Try a different term." />
		{:else}
			<EmptyState title="No clients yet" message="Create your first client to get started.">
				<a href="/clients/new">
					<Button>New Client</Button>
				</a>
			</EmptyState>
		{/if}
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Name</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 sm:table-cell">Email</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 md:table-cell">Phone</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each clients as client}
						<tr class="transition-colors hover:bg-gray-50">
							<td class="px-6 py-4">
								<a href="/clients/{client.id}" class="font-medium text-primary-600 hover:text-primary-700">
									{client.name}
								</a>
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 sm:table-cell">
								{client.email || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 md:table-cell">
								{client.phone || '-'}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
