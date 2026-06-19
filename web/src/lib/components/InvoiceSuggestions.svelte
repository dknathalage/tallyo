<script lang="ts">
	import { goto } from '$app/navigation';
	import * as shiftsApi from '$lib/api/shifts';
	import { shortDate } from '$lib/shifts/format';
	import type { ShiftSuggestion } from '$lib/api/types';

	type Props = {
		suggestions: ShiftSuggestion[];
		/** Resolve a participant id to a display name. */
		nameFor: (participantId: number) => string;
		/** Restrict to a single participant (the profile view). */
		participantId?: number | null;
	};

	let { suggestions, nameFor, participantId = null }: Props = $props();

	const visible = $derived(
		participantId === null
			? suggestions
			: suggestions.filter((s) => s.participantId === participantId)
	);

	let drafting = $state<number | null>(null);
	let error = $state<string | null>(null);

	async function draft(s: ShiftSuggestion): Promise<void> {
		error = null;
		drafting = s.participantId;
		try {
			const inv = await shiftsApi.draftFromShifts(s.ids);
			await goto(`/invoices/${inv.id}`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to draft the invoice.';
		} finally {
			drafting = null;
		}
	}
</script>

{#if visible.length > 0}
	<div class="space-y-2">
		<p class="text-xs font-semibold tracking-wide text-gray-500 uppercase">
			Suggested invoices (from recorded shifts)
		</p>
		{#if error}
			<p class="text-sm text-red-600">{error}</p>
		{/if}
		{#each visible as s (s.participantId)}
			<div
				class="flex items-center gap-4 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3"
			>
				<div class="flex-1 text-sm">
					<span class="font-semibold">{nameFor(s.participantId)}</span> — {s.count} recorded
					{s.count === 1 ? 'shift' : 'shifts'} · {shortDate(s.from)}–{shortDate(s.to)}
				</div>
				<button
					type="button"
					disabled={drafting === s.participantId}
					onclick={() => draft(s)}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{drafting === s.participantId ? 'Drafting…' : 'Draft invoice'}
				</button>
			</div>
		{/each}
	</div>
{/if}
