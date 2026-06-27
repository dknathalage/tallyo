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

	// Stable id so the dialog references its heading via aria-labelledby.
	const titleId = $props.id();

	// The dialog panel, focused when the modal opens so keyboard / screen-reader
	// users land inside the dialog rather than stranded on the page behind it.
	let panel = $state<HTMLDivElement | null>(null);

	$effect(() => {
		if (open && panel) {
			// Prefer the first focusable control; fall back to the panel itself.
			const focusable = panel.querySelector<HTMLElement>(
				'input, select, textarea, button, [href], [tabindex]:not([tabindex="-1"])'
			);
			(focusable ?? panel).focus();
		}
	});

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
			bind:this={panel}
			role="dialog"
			aria-modal="true"
			aria-labelledby={titleId}
			tabindex="-1"
			class="relative z-10 my-8 w-full max-w-2xl rounded-lg border border-gray-200 bg-white shadow-xl focus:outline-none"
		>
			<div class="flex items-center justify-between border-b border-gray-200 px-5 py-3">
				<h2 id={titleId} class="text-base font-semibold text-gray-900">{title}</h2>
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
