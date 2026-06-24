<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Loader from '@lucide/svelte/icons/loader-circle';

	type Variant = 'primary' | 'secondary' | 'danger' | 'ghost';
	type Size = 'sm' | 'md';

	type Props = HTMLButtonAttributes & {
		variant?: Variant;
		size?: Size;
		loading?: boolean;
		href?: string;
		children: Snippet;
	};

	let {
		variant = 'primary',
		size = 'md',
		loading = false,
		href,
		disabled,
		children,
		class: extra = '',
		...rest
	}: Props = $props();

	// ponytail: plain lookup maps, not a class-variance lib. Four variants, two sizes.
	const variants: Record<Variant, string> = {
		primary: 'bg-brand-700 text-onbrand hover:bg-brand-800',
		secondary: 'border border-gray-300 bg-white text-gray-800 hover:bg-gray-100',
		danger: 'bg-red-600 text-onbrand hover:bg-red-700',
		ghost: 'text-gray-700 hover:bg-gray-100'
	};
	const sizes: Record<Size, string> = {
		sm: 'px-2.5 py-1.5 text-xs',
		md: 'px-3.5 py-2 text-sm'
	};

	const base =
		'inline-flex items-center justify-center gap-1.5 rounded-lg font-medium ' +
		'disabled:cursor-not-allowed disabled:opacity-50 disabled:pointer-events-none';

	let cls = $derived(`${base} ${variants[variant]} ${sizes[size]} ${extra}`);
</script>

{#if href}
	<!-- any: rest is typed for <button>; harmless extras are ignored on <a>. -->
	<a {href} class={cls} {...rest as any}>
		{#if loading}<Loader class="size-4 animate-spin" aria-hidden="true" />{/if}
		{@render children()}
	</a>
{:else}
	<button class={cls} disabled={disabled || loading} {...rest}>
		{#if loading}<Loader class="size-4 animate-spin" aria-hidden="true" />{/if}
		{@render children()}
	</button>
{/if}
