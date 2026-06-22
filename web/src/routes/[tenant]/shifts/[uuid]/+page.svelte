<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import { participants } from '$lib/stores/participants.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import type { Shift } from '$lib/api/types';

	const idParam = $derived((page.params.uuid ?? 'new'));

	let loadedShift = $state<Shift | null>(null);
	let loading = $state(false);
	let loadError = $state<string | null>(null);

	// A scheduled shift is being recorded (mirrors the old openRecord flow).
	const recording = $derived(loadedShift?.status === 'scheduled');

	onMount(() => {
		participants.ensureSubscribed();
		void participants.load();
	});

	// Load the target shift on id change. {#key idParam} remounts ShiftForm so its
	// form state resets alongside this load (new → edit, edit → edit).
	$effect(() => {
		const current = idParam;
		loadError = null;
		loadedShift = null;
		if (current === 'new') return;
		void loadShift(current);
	});

	async function loadShift(id: string): Promise<void> {
		loading = true;
		try {
			loadedShift = await shiftsApi.get(id);
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load shift.';
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
	{:else if idParam !== 'new' && loading && loadedShift === null}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else if idParam === 'new' || loadedShift !== null}
		{#key idParam}
			<ShiftForm inline shift={loadedShift} {recording} onsaved={done} oncancel={done} />
		{/key}
	{/if}
</div>
