<script lang="ts">
	import Button from '$lib/components/shared/Button.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let { onexport, onimport }: { onexport: () => void; onimport: (file: File) => void } = $props();

	let fileInput: HTMLInputElement;

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (input.files?.[0]) {
			onimport(input.files[0]);
			input.value = '';
		}
	}
</script>

<div class="flex items-center gap-2">
	<Button variant="secondary" size="sm" onclick={onexport}>{i18n.t('csv.exportCsv')}</Button>
	<Button variant="secondary" size="sm" onclick={() => fileInput.click()}>{i18n.t('csv.import')}</Button>
	<input bind:this={fileInput} type="file" accept=".csv,.xlsx,.xls" class="hidden" onchange={handleFileSelect} />
</div>
