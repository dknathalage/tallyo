<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { participants } from '$lib/stores/participants.svelte';
	import { planManagers } from '$lib/stores/planManagers.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import type { Participant, MgmtType } from '$lib/api/types';

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

	// Client-side search (generic store has no query support).
	let search = $state('');
	const filtered = $derived.by<Participant[]>(() => {
		const q = search.trim().toLowerCase();
		if (q === '') return participants.items;
		return participants.items.filter(
			(p) =>
				p.name.toLowerCase().includes(q) ||
				p.ndisNumber.toLowerCase().includes(q) ||
				p.email.toLowerCase().includes(q)
		);
	});

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editNdis = $state('');
	let editPlanStart = $state('');
	let editPlanEnd = $state('');
	let editMgmtType = $state<MgmtType>('plan');
	let editPlanManager = $state('');
	let editEmail = $state('');
	let editPhone = $state('');
	let editAddress = $state('');
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		participants.ensureSubscribed();
		void participants.load();
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
			await participants.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create participant.';
		} finally {
			creating = false;
		}
	}

	function startEdit(p: Participant): void {
		rowError = null;
		editId = p.id;
		editName = p.name;
		editNdis = p.ndisNumber;
		editPlanStart = p.planStart ? p.planStart.slice(0, 10) : '';
		editPlanEnd = p.planEnd ? p.planEnd.slice(0, 10) : '';
		editMgmtType = p.mgmtType === 'self' ? 'self' : 'plan';
		editPlanManager = p.planManagerId === null ? '' : String(p.planManagerId);
		editEmail = p.email;
		editPhone = p.phone;
		editAddress = p.address;
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(p: Participant): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await participants.crud.update(p.id, {
				name: editName,
				ndisNumber: editNdis,
				planStart: editPlanStart,
				planEnd: editPlanEnd,
				mgmtType: editMgmtType,
				planManagerId: editMgmtType === 'self' ? null : toId(editPlanManager),
				email: editEmail,
				phone: editPhone,
				address: editAddress,
				metadata: p.metadata
			});
			editId = null;
			await participants.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update participant.';
		} finally {
			busy = false;
		}
	}

	async function removeParticipant(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await participants.crud.remove(id);
			await participants.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete participant.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-start justify-between gap-4">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Participants</h1>
				<p class="text-sm text-gray-500">
					NDIS participants you invoice — plan-managed or self-managed.
				</p>
			</div>
			<button
				type="button"
				onclick={openCreate}
				class="shrink-0 rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
			>
				New participant
			</button>
		</div>

		<Modal bind:open={showForm} title="New participant">
			<form class="grid grid-cols-2 gap-3" onsubmit={createParticipant}>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Name</span>
				<input
					type="text"
					bind:value={newName}
					required
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">NDIS number</span>
				<input
					type="text"
					bind:value={newNdis}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Plan start</span>
				<input
					type="date"
					bind:value={newPlanStart}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Plan end</span>
				<input
					type="date"
					bind:value={newPlanEnd}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Management type</span>
				<select
					bind:value={newMgmtType}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="plan">Plan-managed</option>
					<option value="self">Self-managed</option>
				</select>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Plan manager</span>
				<select
					bind:value={newPlanManager}
					disabled={newMgmtType === 'self'}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				>
					<option value="">— none —</option>
					{#each planManagers.items as pm (pm.id)}
						<option value={String(pm.id)}>{pm.name}</option>
					{/each}
				</select>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Email</span>
				<input
					type="email"
					bind:value={newEmail}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Phone</span>
				<input
					type="text"
					bind:value={newPhone}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-2">
				<span class="mb-1 block text-sm font-medium">Address</span>
				<input
					type="text"
					bind:value={newAddress}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			{#if formError}
				<p class="col-span-2 text-sm text-red-600">{formError}</p>
			{/if}
			<div class="col-span-2 flex gap-2">
				<button
					type="submit"
					disabled={creating}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{creating ? 'Adding…' : 'Add participant'}
				</button>
				<button
					type="button"
					onclick={cancelCreate}
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Cancel
				</button>
			</div>
			</form>
		</Modal>
	</section>

	<section>
		<label class="mb-4 block max-w-sm">
			<span class="mb-1 block text-sm font-medium">Search</span>
			<input
				type="text"
				bind:value={search}
				placeholder="Filter by name, NDIS number or email"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		{#if participants.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if participants.error}
			<p class="text-sm text-red-600">{participants.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Name</th>
						<th class="px-3 py-2 font-medium">NDIS #</th>
						<th class="px-3 py-2 font-medium">Plan window</th>
						<th class="px-3 py-2 font-medium">Management</th>
						<th class="px-3 py-2 font-medium">Plan manager</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as p (p.id)}
						<tr class="border-b border-gray-100 last:border-0">
							{#if editId === p.id}
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editName}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editNdis}
										class="w-28 rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<div class="flex flex-col gap-1">
										<input
											type="date"
											bind:value={editPlanStart}
											class="rounded border border-gray-300 px-2 py-1 text-sm"
										/>
										<input
											type="date"
											bind:value={editPlanEnd}
											class="rounded border border-gray-300 px-2 py-1 text-sm"
										/>
									</div>
								</td>
								<td class="px-3 py-2">
									<select
										bind:value={editMgmtType}
										class="rounded border border-gray-300 px-2 py-1 text-sm"
									>
										<option value="plan">Plan</option>
										<option value="self">Self</option>
									</select>
								</td>
								<td class="px-3 py-2">
									<select
										bind:value={editPlanManager}
										disabled={editMgmtType === 'self'}
										class="rounded border border-gray-300 px-2 py-1 text-sm disabled:bg-gray-100"
									>
										<option value="">— none —</option>
										{#each planManagers.items as pm (pm.id)}
											<option value={String(pm.id)}>{pm.name}</option>
										{/each}
									</select>
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										disabled={busy}
										onclick={() => saveEdit(p)}
										class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
									>
										Save
									</button>
									<button type="button" onclick={cancelEdit} class="text-gray-500 hover:underline">
										Cancel
									</button>
								</td>
							{:else}
								<td class="px-3 py-2 font-medium">{p.name}</td>
								<td class="px-3 py-2 text-gray-600">{p.ndisNumber || '—'}</td>
								<td class="px-3 py-2 text-gray-600">
									{p.planStart ? p.planStart.slice(0, 10) : '—'} – {p.planEnd
										? p.planEnd.slice(0, 10)
										: '—'}
								</td>
								<td class="px-3 py-2 text-gray-600 capitalize">
									{p.mgmtType === 'self' ? 'Self-managed' : 'Plan-managed'}
								</td>
								<td class="px-3 py-2 text-gray-600">{p.planManagerName || '—'}</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										onclick={() => goto(`/participants/${p.id}`)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Open
									</button>
									<button
										type="button"
										onclick={() => startEdit(p)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={busy}
										onclick={() => removeParticipant(p.id)}
										class="text-red-600 hover:underline disabled:opacity-50"
									>
										Delete
									</button>
								</td>
							{/if}
						</tr>
					{:else}
						<tr>
							<td colspan="6" class="px-3 py-6 text-center text-gray-500">
								No participants found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>

</div>
