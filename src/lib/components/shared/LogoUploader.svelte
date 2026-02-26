<script lang="ts">
	let {
		logo = $bindable()
	}: {
		logo: string;
	} = $props();

	let error = $state('');

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		error = '';

		if (!file.type.startsWith('image/')) {
			error = 'Please select an image file';
			return;
		}

		if (file.size > 500 * 1024) {
			error = 'Image must be under 500KB';
			return;
		}

		const reader = new FileReader();
		reader.onload = () => {
			logo = reader.result as string;
		};
		reader.readAsDataURL(file);
	}

	function removeLogo() {
		logo = '';
	}
</script>

<div class="space-y-2">
	{#if logo}
		<div class="flex items-start gap-3">
			<img src={logo} alt="Logo preview" class="h-16 w-16 rounded-lg border border-gray-200 object-contain dark:border-gray-700" />
			<button
				type="button"
				onclick={removeLogo}
				class="cursor-pointer text-sm text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300"
			>
				Remove
			</button>
		</div>
	{:else}
		<label class="flex cursor-pointer items-center gap-2 rounded-lg border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 transition-colors hover:border-gray-400 hover:text-gray-600 dark:border-gray-600 dark:text-gray-400 dark:hover:border-gray-500 dark:hover:text-gray-300">
			<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.41a2.25 2.25 0 013.182 0l2.909 2.91M3.75 21h16.5A2.25 2.25 0 0022.5 18.75V5.25A2.25 2.25 0 0020.25 3H3.75A2.25 2.25 0 001.5 5.25v13.5A2.25 2.25 0 003.75 21z" />
			</svg>
			Upload logo (max 500KB)
			<input type="file" accept="image/*" onchange={handleFileSelect} class="hidden" />
		</label>
	{/if}
	{#if error}
		<p class="text-sm text-red-600 dark:text-red-400">{error}</p>
	{/if}
</div>
