<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import { clients } from '$lib/stores/clients.svelte';
	import * as sessionsApi from '$lib/api/sessions';
	import SessionForm from '$lib/components/SessionForm.svelte';
	import type { Session } from '$lib/api/types';

	const idParam = $derived((page.params.uuid ?? 'new'));

	let loadedSession = $state<Session | null>(null);
	let loading = $state(false);
	let loadError = $state<string | null>(null);

	// A scheduled session is being recorded (mirrors the old openRecord flow).
	const recording = $derived(loadedSession?.status === 'scheduled');

	onMount(() => {
		clients.ensureSubscribed();
		void clients.load();
	});

	// Load the target session on id change. {#key idParam} remounts SessionForm so its
	// form state resets alongside this load. Creation is modal-only now, so a stray
	// /sessions/new redirects back to the dashboard.
	$effect(() => {
		const current = idParam;
		loadError = null;
		loadedSession = null;
		if (current === 'new') {
			void goto(t('/'));
			return;
		}
		void loadSession(current);
	});

	async function loadSession(id: string): Promise<void> {
		loading = true;
		try {
			loadedSession = await sessionsApi.get(id);
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load session.';
		} finally {
			loading = false;
		}
	}

	function done(): void {
		void goto(t('/'));
	}
</script>

<div class="space-y-4">
	<a href={t('/')} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>

	{#if loadError}
		<p class="text-sm text-red-600">{loadError}</p>
	{:else if idParam !== 'new' && loading && loadedSession === null}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else if loadedSession !== null}
		{#key idParam}
			<SessionForm inline session={loadedSession} {recording} onsaved={done} oncancel={done} />
		{/key}
	{/if}
</div>
