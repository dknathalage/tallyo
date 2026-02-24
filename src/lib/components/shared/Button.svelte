<script lang="ts">
	import type { Snippet } from 'svelte';

	type Variant = 'primary' | 'secondary' | 'danger' | 'ghost';
	type Size = 'sm' | 'md' | 'lg';

	let {
		variant = 'primary',
		size = 'md',
		disabled = false,
		type = 'button',
		onclick,
		class: className = '',
		children
	}: {
		variant?: Variant;
		size?: Size;
		disabled?: boolean;
		type?: 'button' | 'submit';
		onclick?: (e: MouseEvent) => void;
		class?: string;
		children: Snippet;
	} = $props();

	const variantClasses: Record<Variant, string> = {
		primary: 'bg-primary-600 text-white hover:bg-primary-700 focus:ring-primary-500',
		secondary: 'bg-white text-gray-700 border border-gray-300 hover:bg-gray-50 focus:ring-primary-500',
		danger: 'bg-red-600 text-white hover:bg-red-700 focus:ring-red-500',
		ghost: 'bg-transparent text-gray-700 hover:bg-gray-100 focus:ring-primary-500'
	};

	const sizeClasses: Record<Size, string> = {
		sm: 'px-3 py-1.5 text-sm',
		md: 'px-4 py-2 text-sm',
		lg: 'px-6 py-3 text-lg'
	};
</script>

<button
	{type}
	{disabled}
	{onclick}
	class="inline-flex cursor-pointer items-center justify-center rounded-lg font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 {variantClasses[variant]} {sizeClasses[size]} {disabled ? 'cursor-not-allowed opacity-50' : ''} {className}"
>
	{@render children()}
</button>
