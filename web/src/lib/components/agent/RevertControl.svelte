<script lang="ts">
	import { agentChat } from '$lib/stores/agentChat.svelte';

	interface Props {
		checkpointId: number;
		status?: string;
	}

	let { checkpointId, status }: Props = $props();

	// UI phases: 'idle' shows the revert button, 'confirm' the inline two-step
	// confirm, 'reverting' while the request is in flight, 'done' after it
	// resolves (success or with conflicts).
	type Phase = 'idle' | 'confirm' | 'reverting' | 'done';
	let phase = $state<Phase>('idle');

	// Conflicts captured from the store's lastRevert at the moment our revert
	// resolved. Null until 'done'. We snapshot rather than read the store
	// reactively so a later unrelated revert can't rewrite this control's result.
	let conflicts = $state<{ table: string; pk: number }[] | null>(null);

	// Only a committed checkpoint is revertable; a reverted one shows a label.
	const canRevert = $derived(status === 'committed');
	const alreadyReverted = $derived(status === 'reverted');

	function ask(): void {
		if (!canRevert) return;
		phase = 'confirm';
	}

	function cancel(): void {
		phase = 'idle';
	}

	async function confirm(): Promise<void> {
		if (phase === 'reverting') return;
		phase = 'reverting';
		await agentChat.revert(checkpointId);
		// The store stashes the API result on lastRevert; snapshot its conflicts.
		const result = agentChat.lastRevert;
		conflicts = result !== null ? result.conflicts : [];
		phase = 'done';
	}

	const conflictCount = $derived(conflicts !== null ? conflicts.length : 0);
</script>

{#if alreadyReverted}
	<span class="text-xs text-gray-400 italic">Reverted</span>
{:else if canRevert}
	{#if phase === 'idle'}
		<button
			type="button"
			class="text-xs font-medium text-gray-500 underline-offset-2 hover:text-red-600 hover:underline"
			onclick={ask}
		>
			Revert this change
		</button>
	{:else if phase === 'confirm'}
		<span class="inline-flex items-center gap-2 text-xs text-gray-600">
			<span>Revert this change?</span>
			<button
				type="button"
				class="rounded bg-red-600 px-2 py-0.5 font-medium text-white hover:bg-red-700"
				onclick={confirm}
			>
				Yes, revert
			</button>
			<button
				type="button"
				class="rounded border border-gray-300 px-2 py-0.5 font-medium text-gray-600 hover:bg-gray-50"
				onclick={cancel}
			>
				Cancel
			</button>
		</span>
	{:else if phase === 'reverting'}
		<span class="text-xs text-gray-400" aria-live="polite">Reverting…</span>
	{:else}
		<!-- phase === 'done' -->
		{#if conflictCount === 0}
			<span class="text-xs font-medium text-green-700" aria-live="polite">Change reverted</span>
		{:else}
			<span class="text-xs font-medium text-amber-700" aria-live="polite">
				{conflictCount} change{conflictCount === 1 ? '' : 's'} skipped — edited since
			</span>
		{/if}
	{/if}
{/if}
