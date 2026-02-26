<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fade, scale } from 'svelte/transition';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		open = false,
		onclose,
		title,
		maxWidth = 'max-w-lg',
		children
	}: {
		open: boolean;
		onclose: () => void;
		title: string;
		maxWidth?: string;
		children: Snippet;
	} = $props();

	let dialogEl: HTMLDivElement | undefined = $state();
	let previouslyFocusedEl: HTMLElement | null = null;

	const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

	function getFocusableElements(): HTMLElement[] {
		if (!dialogEl) return [];
		return Array.from(dialogEl.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
	}

	$effect(() => {
		if (open) {
			// Store the element that had focus before the modal opened
			previouslyFocusedEl = document.activeElement as HTMLElement | null;

			// Move focus into the modal after DOM renders
			requestAnimationFrame(() => {
				const focusable = getFocusableElements();
				if (focusable.length > 0) {
					focusable[0].focus();
				} else if (dialogEl) {
					dialogEl.focus();
				}
			});
		} else {
			// Restore focus to the previously focused element when modal closes
			if (previouslyFocusedEl && typeof previouslyFocusedEl.focus === 'function') {
				previouslyFocusedEl.focus();
				previouslyFocusedEl = null;
			}
		}
	});

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			onclose();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onclose();
			return;
		}

		// Focus trap: cycle Tab/Shift+Tab within the modal
		if (e.key === 'Tab') {
			const focusable = getFocusableElements();
			if (focusable.length === 0) {
				e.preventDefault();
				return;
			}

			const first = focusable[0];
			const last = focusable[focusable.length - 1];

			if (e.shiftKey) {
				// Shift+Tab: if focus is on first element, wrap to last
				if (document.activeElement === first) {
					e.preventDefault();
					last.focus();
				}
			} else {
				// Tab: if focus is on last element, wrap to first
				if (document.activeElement === last) {
					e.preventDefault();
					first.focus();
				}
			}
		}
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center p-4"
		onkeydown={handleKeydown}
	>
		<!-- Backdrop -->
		<div
			class="absolute inset-0 bg-black/50"
			onclick={handleBackdropClick}
			onkeydown={handleKeydown}
			role="presentation"
			transition:fade={{ duration: 150 }}
		></div>

		<!-- Dialog -->
		<div
			bind:this={dialogEl}
			class="relative z-10 w-full {maxWidth} rounded-xl bg-white shadow-2xl dark:bg-gray-800"
			transition:scale={{ duration: 150, start: 0.95 }}
			role="dialog"
			aria-modal="true"
			aria-label={title}
		>
			<!-- Header -->
			<div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{title}</h2>
				<button
					onclick={onclose}
					class="cursor-pointer rounded-md p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:text-gray-500 dark:hover:bg-gray-700 dark:hover:text-gray-300"
					aria-label={i18n.t('a11y.closeModal')}
				>
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<!-- Body -->
			<div class="px-6 py-4">
				{@render children()}
			</div>
		</div>
	</div>
{/if}
