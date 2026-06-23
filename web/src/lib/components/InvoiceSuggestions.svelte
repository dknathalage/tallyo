<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import * as shiftsApi from '$lib/api/shifts';
	import { shortDate } from '$lib/shifts/format';
	import type { ShiftSuggestion } from '$lib/api/types';

	type Props = {
		suggestions: ShiftSuggestion[];
		/** Resolve a client uuid to a display name. */
		nameFor: (clientId: string) => string;
		/** Restrict to a single client (the profile view), by uuid. */
		clientId?: string | null;
	};

	let { suggestions, nameFor, clientId = null }: Props = $props();

	const visible = $derived(
		clientId === null
			? suggestions
			: suggestions.filter((s) => s.clientId === clientId)
	);

	let drafting = $state<string | null>(null);
	let error = $state<string | null>(null);

	async function draft(s: ShiftSuggestion): Promise<void> {
		error = null;
		drafting = s.clientId;
		try {
			const inv = await shiftsApi.draftFromShifts(s.ids);
			await goto(t(`/invoices/${inv.id}`));
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
		{#each visible as s (s.clientId)}
			<div
				class="flex items-center gap-4 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3"
			>
				<div class="flex-1 text-sm">
					<span class="font-semibold">{nameFor(s.clientId)}</span> — {s.count} recorded
					{s.count === 1 ? 'shift' : 'shifts'} · {shortDate(s.from)}–{shortDate(s.to)}
				</div>
				<button
					type="button"
					disabled={drafting === s.clientId}
					onclick={() => draft(s)}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{drafting === s.clientId ? 'Drafting…' : 'Draft invoice'}
				</button>
			</div>
		{/each}
	</div>
{/if}
