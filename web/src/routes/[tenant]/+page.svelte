<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import { sessions } from '$lib/stores/sessions.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import * as sessionsApi from '$lib/api/sessions';
	import SessionTable from '$lib/components/SessionTable.svelte';
	import SessionForm from '$lib/components/SessionForm.svelte';
	import Button from '$lib/components/Button.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import { shortDate, todayISO } from '$lib/sessions/format';
	import type { Session } from '$lib/api/types';

	onMount(() => {
		sessions.ensureSubscribed();
		void sessions.load();
		clients.ensureSubscribed();
		void clients.load();
	});

	function clientName(id: string): string {
		const p = clients.items.find((x) => x.id === id);
		return p ? p.name : `#${id}`;
	}

	const today = todayISO();
	function whenLabel(date: string): string {
		if (date < today) return '⚠ Overdue';
		if (date === today) return 'Today';
		return 'Upcoming';
	}

	// ---- Session editing (record / edit) — full route pages; add via modal. ----
	let addOpen = $state(false);

	function openSession(session: Session): void {
		void goto(t('/sessions/' + session.id));
	}

	async function deleteSessions(ids: string[]): Promise<void> {
		for (const id of ids) {
			await sessionsApi.remove(id);
		}
		await sessions.load();
	}
</script>

<div class="space-y-6">
	<div class="flex items-start justify-between gap-4">
		<div>
			<h1 class="mb-1 text-2xl font-semibold tracking-tight">Sessions</h1>
			<p class="text-sm text-gray-500">
				Record scheduled sessions, then draft invoices from recorded work.
			</p>
		</div>
		<Button class="shrink-0" onclick={() => (addOpen = true)}>+ Add session</Button>
	</div>

	{#if sessions.error}
		<p class="text-sm text-red-600">{sessions.error}</p>
	{/if}

	<!-- Sessions to record -->
	{#if sessions.toRecord.length > 0}
		<section class="rounded-xl border border-amber-200 bg-white p-4 shadow-sm" aria-label="Sessions to record">
			<h2 class="mb-3 text-xs font-semibold tracking-wide text-amber-700 uppercase">
				⏱ Sessions to record ({sessions.toRecord.length})
			</h2>
			<div class="space-y-2">
				{#each sessions.toRecord as s (s.id)}
					<div class="flex flex-wrap items-center gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2">
						<span class="min-w-[8rem] font-semibold">{whenLabel(s.serviceDate)} · {shortDate(s.serviceDate)}</span>
						<span class="flex flex-1 items-center gap-2 text-sm">
							{clientName(s.clientId)}
							<Badge tone="amber">scheduled</Badge>
						</span>
						<Button size="sm" onclick={() => goto(t('/sessions/' + s.id))}>Record session →</Button>
					</div>
				{/each}
			</div>
			<p class="mt-2 text-xs text-gray-500">
				Tallyo asks you to record each scheduled session — add a note, hours/time, distance and other
				measures.
			</p>
		</section>
	{/if}


	{#if sessions.loading && sessions.items.length === 0}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else}
		<section>
			<SessionTable sessions={sessions.items} {clientName} onopen={openSession} ondelete={deleteSessions} />
			<p class="mt-2 text-xs text-gray-500">
				Status pipeline: scheduled → recorded → drafted → sent → paid.
			</p>
		</section>
	{/if}
</div>

<SessionForm bind:open={addOpen} onsaved={() => sessions.load()} />
