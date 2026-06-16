<script lang="ts">
	import type { PlanStepDTO } from '$lib/api/agent';

	interface Props {
		steps: PlanStepDTO[];
	}

	let { steps }: Props = $props();

	function riskClass(risk: PlanStepDTO['risk']): string {
		if (risk === 'risky') return 'bg-amber-100 text-amber-800 border border-amber-300';
		if (risk === 'meta') return 'bg-gray-100 text-gray-500 border border-gray-200';
		// 'read' — neutral gray
		return 'bg-gray-100 text-gray-700 border border-gray-200';
	}

	function riskLabel(risk: PlanStepDTO['risk']): string {
		if (risk === 'risky') return 'risky';
		if (risk === 'meta') return 'meta';
		return 'read';
	}
</script>

<div class="rounded-lg border border-gray-200 bg-white px-4 py-3">
	<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">Plan</h3>
	<ol class="space-y-2">
		{#each steps as step, i (i)}
			<li class="flex items-start gap-2 text-sm">
				<span class="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-gray-100 text-xs font-medium text-gray-600">
					{i + 1}
				</span>
				<span class="flex-1 text-gray-800">{step.summary}</span>
				<span
					class="shrink-0 rounded px-1.5 py-0.5 text-xs font-medium {riskClass(step.risk)}"
					aria-label="risk: {riskLabel(step.risk)}"
				>
					{riskLabel(step.risk)}
				</span>
			</li>
		{/each}
	</ol>
</div>
