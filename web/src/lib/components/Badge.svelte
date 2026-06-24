<script lang="ts">
	import type { Snippet } from 'svelte';

	type Tone = 'gray' | 'brand' | 'green' | 'amber' | 'red' | 'blue' | 'slate';

	type Props = {
		children: Snippet;
		tone?: Tone;
		class?: string;
	};

	let { children, tone = 'gray', class: extra = '' }: Props = $props();

	// ponytail: static map — these are the only status tones the app uses.
	const tones: Record<Tone, string> = {
		gray: 'bg-gray-100 text-gray-700 ring-gray-200',
		brand: 'bg-brand-50 text-brand-700 ring-brand-200',
		green: 'bg-green-50 text-green-700 ring-green-200',
		amber: 'bg-amber-50 text-amber-700 ring-amber-200',
		red: 'bg-red-50 text-red-700 ring-red-200',
		blue: 'bg-blue-50 text-blue-700 ring-blue-200',
		slate: 'bg-slate-50 text-slate-700 ring-slate-200'
	};
</script>

<span
	class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ring-1 {tones[
		tone
	]} {extra}"
>
	{@render children()}
</span>
