<script lang="ts">
	import { onMount } from 'svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { planManagers } from '$lib/stores/planManagers.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { Participant, ParticipantInput, MgmtType } from '$lib/api/types';

	// Selects bind to a string id ('' means none); convert to number | null.
	function toId(v: string): number | null {
		return v === '' ? null : Number(v);
	}

	// New-participant form fields.
	let newName = $state('');
	let newNdis = $state('');
	let newPlanStart = $state('');
	let newPlanEnd = $state('');
	let newMgmtType = $state<MgmtType>('plan');
	let newPlanManager = $state('');
	let newEmail = $state('');
	let newPhone = $state('');
	let newAddress = $state('');
	let creating = $state(false);
	let formError = $state<string | null>(null);
	let showForm = $state(false);

	function openCreate(): void {
		resetNew();
		formError = null;
		showForm = true;
	}
	function cancelCreate(): void {
		resetNew();
		formError = null;
		showForm = false;
	}

	onMount(() => {
		participants.ensureSubscribed();
		void participants.query({ page: 1, limit: 50 });
		planManagers.ensureSubscribed();
		void planManagers.load();
	});

	function resetNew(): void {
		newName = '';
		newNdis = '';
		newPlanStart = '';
		newPlanEnd = '';
		newMgmtType = 'plan';
		newPlanManager = '';
		newEmail = '';
		newPhone = '';
		newAddress = '';
	}

	async function createParticipant(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await participants.crud.create({
				name: newName,
				ndisNumber: newNdis,
				planStart: newPlanStart,
				planEnd: newPlanEnd,
				mgmtType: newMgmtType,
				planManagerId: newMgmtType === 'self' ? null : toId(newPlanManager),
				email: newEmail,
				phone: newPhone,
				address: newAddress,
				metadata: ''
			});
			resetNew();
			showForm = false;
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create participant.';
		} finally {
			creating = false;
		}
	}

	// DataTable column definitions. Keys match Participant JSON fields (and the
	// server allowlist), so one key drives filter, sort, display, and drawer edit.
	const columns: Column<Participant>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'ndisNumber', label: 'NDIS #', sortable: true, filter: 'text' },
		{
			key: 'planStart',
			label: 'Plan start',
			sortable: true,
			filter: 'date',
			cell: (p) => (p.planStart ? p.planStart.slice(0, 10) : '—')
		},
		{
			key: 'planEnd',
			label: 'Plan end',
			sortable: true,
			filter: 'date',
			cell: (p) => (p.planEnd ? p.planEnd.slice(0, 10) : '—')
		},
		{
			key: 'mgmtType',
			label: 'Management',
			sortable: true,
			filter: 'enum',
			values: ['plan', 'self'],
			cell: (p) => (p.mgmtType === 'self' ? 'Self-managed' : 'Plan-managed')
		},
		{ key: 'planManagerName', label: 'Plan manager', sortable: true, filter: 'text' }
	];

	// Map a (possibly drawer-edited) Participant back to its writable input.
	function toInput(p: Participant): ParticipantInput {
		return {
			name: p.name,
			ndisNumber: p.ndisNumber,
			planStart: p.planStart,
			planEnd: p.planEnd,
			mgmtType: p.mgmtType === 'self' ? 'self' : 'plan',
			planManagerId: p.planManagerId,
			email: p.email,
			phone: p.phone,
			address: p.address,
			metadata: p.metadata
		};
	}

	const rowActions: RowAction<Participant>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await participants.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Participants</h1>
			<p class="text-sm text-gray-500">
				NDIS participants you invoice — plan-managed or self-managed.
			</p>
		</div>

		<Modal bind:open={showForm} title="New participant">
			<form class="grid grid-cols-2 gap-3" onsubmit={createParticipant}>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Name</span>
					<input type="text" bind:value={newName} required class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">NDIS number</span>
					<input type="text" bind:value={newNdis} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Plan start</span>
					<input type="date" bind:value={newPlanStart} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Plan end</span>
					<input type="date" bind:value={newPlanEnd} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Management type</span>
					<select bind:value={newMgmtType} class="w-full rounded border border-gray-300 px-3 py-2 text-sm">
						<option value="plan">Plan-managed</option>
						<option value="self">Self-managed</option>
					</select>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Plan manager</span>
					<select bind:value={newPlanManager} disabled={newMgmtType === 'self'} class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100">
						<option value="">— none —</option>
						{#each planManagers.items as pm (pm.id)}
							<option value={String(pm.id)}>{pm.name}</option>
						{/each}
					</select>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Email</span>
					<input type="email" bind:value={newEmail} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Phone</span>
					<input type="text" bind:value={newPhone} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-2">
					<span class="mb-1 block text-sm font-medium">Address</span>
					<input type="text" bind:value={newAddress} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				{#if formError}
					<p class="col-span-2 text-sm text-red-600">{formError}</p>
				{/if}
				<div class="col-span-2 flex gap-2">
					<button type="submit" disabled={creating} class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
						{creating ? 'Adding…' : 'Add participant'}
					</button>
					<button type="button" onclick={cancelCreate} class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50">Cancel</button>
				</div>
			</form>
		</Modal>
	</section>

	<section>
		{#if participants.error}
			<p class="mb-3 text-sm text-red-600">{participants.error}</p>
		{/if}

		<DataTable
			title="Participants"
			{columns}
			store={participants}
			{rowActions}
			onNew={openCreate}
			onRowSave={async (row) => {
				await participants.crud.update(row.id, toInput(row));
			}}
			detailHref={(p) => `/participants/${p.id}`}
		/>
	</section>
</div>
