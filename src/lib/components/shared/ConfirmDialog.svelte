<script lang="ts">
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';

	let {
		open = false,
		title = 'Confirm',
		message = '',
		confirmLabel = 'Confirm',
		confirmVariant = 'primary',
		onconfirm,
		oncancel
	}: {
		open: boolean;
		title?: string;
		message: string;
		confirmLabel?: string;
		confirmVariant?: 'primary' | 'danger';
		onconfirm: () => void;
		oncancel: () => void;
	} = $props();
</script>

<Modal {open} onclose={oncancel} {title}>
	<p class="text-sm text-gray-600">{message}</p>
	<div class="mt-6 flex justify-end gap-3">
		<Button variant="secondary" onclick={oncancel}>Cancel</Button>
		<Button variant={confirmVariant} onclick={onconfirm}>{confirmLabel}</Button>
	</div>
</Modal>
