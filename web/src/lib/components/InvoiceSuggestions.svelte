<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import * as sessionsApi from '$lib/api/sessions';
	import { shortDate } from '$lib/sessions/format';
	import Button from '$lib/components/Button.svelte';
	import type { SessionSuggestion } from '$lib/api/types';

	type Props = {
		suggestions: SessionSuggestion[];
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

	async function draft(s: SessionSuggestion): Promise<void> {
		error = null;
		drafting = s.clientId;
		try {
			const inv = await sessionsApi.draftFromSessions(s.ids);
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
			Suggested invoices (from recorded sessions)
		</p>
		{#if error}
			<p class="text-sm text-red-600">{error}</p>
		{/if}
		{#each visible as s (s.clientId)}
			<div
				class="flex items-center gap-4 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3"
			>
				<div class="flex-1 text-sm">
					<span class="font-semibold">{nameFor(s.clientId)}</span> — <span class="font-mono tabular-nums">{s.count}</span> recorded
					{s.count === 1 ? 'session' : 'sessions'} · <span class="font-mono tabular-nums">{shortDate(s.from)}–{shortDate(s.to)}</span>
				</div>
				<Button
					loading={drafting === s.clientId}
					disabled={drafting === s.clientId}
					onclick={() => draft(s)}
				>
					{drafting === s.clientId ? 'Drafting…' : 'Draft invoice'}
				</Button>
			</div>
		{/each}
	</div>
{/if}
