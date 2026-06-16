<script lang="ts">
	interface Props {
		value: unknown;
	}

	let { value }: Props = $props();

	const display = $derived(String(value ?? ''));
</script>

<p class="whitespace-pre-wrap break-all text-sm text-gray-800">{display}</p>
