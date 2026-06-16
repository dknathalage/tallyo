<script lang="ts">
	import type { ToolResultView as ToolResultViewType } from '$lib/stores/agentChatReducer';
	import { chooseRenderer } from './resultRender';
	import ResultTable from './ResultTable.svelte';
	import ResultCard from './ResultCard.svelte';
	import ResultSummary from './ResultSummary.svelte';

	interface Props {
		result: ToolResultViewType;
	}

	let { result }: Props = $props();

	const kind = $derived(chooseRenderer(result.render, result.result, result.isError));

	/**
	 * Safely coerce result.result to Record<string, unknown>[] for the table renderer.
	 * The chooseRenderer already verified it's a non-empty array of plain objects,
	 * but TypeScript doesn't know that here so we guard defensively.
	 */
	function asTableRows(value: unknown): Record<string, unknown>[] {
		if (!Array.isArray(value)) return [];
		return value.filter(
			(item): item is Record<string, unknown> =>
				typeof item === 'object' && item !== null && !Array.isArray(item)
		);
	}

	/**
	 * Safely coerce result.result to Record<string, unknown> for the card renderer.
	 */
	function asCardData(value: unknown): Record<string, unknown> {
		if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
			return value as Record<string, unknown>;
		}
		return {};
	}
</script>

<div class="mt-1 space-y-1">
	<p class="text-xs font-medium text-gray-400">Tool result</p>

	{#if kind === 'error'}
		<div class="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
			{result.error ?? 'Unknown error'}
		</div>
	{:else if kind === 'table'}
		<ResultTable rows={asTableRows(result.result)} />
	{:else if kind === 'card'}
		<ResultCard data={asCardData(result.result)} />
	{:else}
		<ResultSummary value={result.result} />
	{/if}
</div>
