<script lang="ts">
	import { tableColumns, formatCell } from './resultRender';

	interface Props {
		rows: Record<string, unknown>[];
	}

	let { rows }: Props = $props();

	const columns = $derived(tableColumns(rows));

	function isNumeric(value: unknown): boolean {
		return typeof value === 'number' || (typeof value === 'string' && value.trim() !== '' && !isNaN(Number(value)));
	}
</script>

<div class="overflow-x-auto rounded border border-gray-200">
	<table class="min-w-full text-sm">
		<thead>
			<tr class="border-b border-gray-200 bg-gray-50">
				{#each columns as col (col)}
					<th class="px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
						{col}
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as row, i (i)}
				<tr class="border-b border-gray-100 last:border-0 {i % 2 === 0 ? 'bg-white' : 'bg-gray-50'}">
					{#each columns as col (col)}
						<td
							class="px-3 py-2 text-gray-800 {isNumeric(row[col]) ? 'text-right tabular-nums' : ''}"
						>
							{formatCell(row[col])}
						</td>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
</div>
