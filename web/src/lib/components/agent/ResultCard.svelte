<script lang="ts">
	interface Props {
		data: Record<string, unknown>;
	}

	let { data }: Props = $props();

	const entries = $derived(Object.entries(data));
</script>

<dl class="divide-y divide-gray-100 rounded border border-gray-200 bg-white">
	{#each entries as [key, value] (key)}
		<div class="flex gap-3 px-3 py-2 text-sm">
			<dt class="w-1/3 shrink-0 font-medium text-gray-500">{key}</dt>
			<dd class="flex-1 break-all text-gray-800">{String(value ?? '')}</dd>
		</div>
	{/each}
</dl>
