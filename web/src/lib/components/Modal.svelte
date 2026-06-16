<script lang="ts">
	import type { Snippet } from 'svelte';
	import X from '@lucide/svelte/icons/x';

	type Props = {
		open: boolean;
		title: string;
		children: Snippet;
		footer?: Snippet;
	};

	let { open = $bindable(), title, children, footer }: Props = $props();

	function close(): void {
		open = false;
	}

	function onKeydown(e: KeyboardEvent): void {
		if (open && e.key === 'Escape') close();
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
	<div class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto p-4 sm:items-center">
		<!-- Backdrop -->
		<button
			type="button"
			aria-label="Close"
			class="fixed inset-0 cursor-default bg-black/50"
			onclick={close}
		></button>

		<!-- Panel -->
		<div
			role="dialog"
			aria-modal="true"
			aria-label={title}
			class="relative z-10 my-8 w-full max-w-2xl rounded-lg border border-gray-200 bg-white shadow-xl"
		>
			<div class="flex items-center justify-between border-b border-gray-200 px-5 py-3">
				<h2 class="text-base font-semibold text-gray-900">{title}</h2>
				<button
					type="button"
					onclick={close}
					aria-label="Close"
					class="rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900"
				>
					<X class="size-5" aria-hidden="true" />
				</button>
			</div>

			<div class="max-h-[70vh] overflow-y-auto px-5 py-4">
				{@render children()}
			</div>

			{#if footer}
				<div class="flex justify-end gap-2 border-t border-gray-200 px-5 py-3">
					{@render footer()}
				</div>
			{/if}
		</div>
	</div>
{/if}
