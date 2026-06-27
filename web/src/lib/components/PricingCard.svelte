<script lang="ts">
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';

	type Props = {
		name: string;
		price: string;
		period: string;
		description: string;
		features: string[];
		popular?: boolean;
	};

	let {
		name,
		price,
		period,
		description,
		features,
		popular = false
	}: Props = $props();
</script>

<div
	class="relative flex flex-col rounded-xl border p-6 shadow-sm
		{popular
		? 'border-brand-500 bg-brand-700 text-onbrand ring-2 ring-brand-500'
		: 'border-gray-200 bg-white text-gray-900'}"
>
	{#if popular}
		<div class="absolute -top-3 left-1/2 -translate-x-1/2">
			<Badge tone="amber">Most popular</Badge>
		</div>
	{/if}

	<div class="mb-4">
		<h3 class="text-lg font-semibold {popular ? 'text-onbrand dark:text-white' : 'text-gray-900'}">
			{name}
		</h3>
		<p class="mt-1 text-sm {popular ? 'text-brand-100 dark:text-white' : 'text-gray-500'}">
			{description}
		</p>
	</div>

	<div class="mb-6">
		<span
			class="font-mono text-4xl font-bold tabular-nums {popular
				? 'text-onbrand dark:text-white'
				: 'text-gray-900'}">{price}</span
		>
		<span class="ml-1 text-sm {popular ? 'text-brand-100 dark:text-white' : 'text-gray-500'}"
			>{period}</span
		>
	</div>

	<ul class="mb-8 flex-1 space-y-2.5" aria-label="Plan features">
		{#each features as feature}
			<li class="flex items-start gap-2 text-sm">
				<svg
					class="mt-0.5 size-4 shrink-0 {popular ? 'text-brand-200' : 'text-brand-600'}"
					viewBox="0 0 20 20"
					fill="currentColor"
					aria-hidden="true"
				>
					<path
						fill-rule="evenodd"
						d="M16.704 4.153a.75.75 0 0 1 .143 1.052l-8 10.5a.75.75 0 0 1-1.127.075l-4.5-4.5a.75.75 0 0 1 1.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 0 1 1.05-.143Z"
						clip-rule="evenodd"
					/>
				</svg>
				<span class="{popular ? 'text-brand-50 dark:text-white' : 'text-gray-600'}">{feature}</span>
			</li>
		{/each}
	</ul>

	<Button
		href="/signup"
		variant={popular ? 'secondary' : 'primary'}
		class="w-full {popular ? 'bg-white text-brand-700 hover:bg-brand-50' : ''}"
	>
		Start free trial
	</Button>
</div>
