<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import { db, initNew, openExisting, tryRestore, reconnect } from '$lib/db/connection.svelte';

	let { children }: { children: Snippet } = $props();
	let restoring = $state(true);
	let storedFileName: string | null = $state(null);

	onMount(async () => {
		storedFileName = await tryRestore();
		restoring = false;
	});
</script>

{#if restoring}
	<div class="flex min-h-screen items-center justify-center bg-gradient-to-br from-primary-50 via-white to-primary-100">
		<div class="text-center">
			<div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-xl bg-primary-600 text-2xl font-bold text-white">
				IM
			</div>
			<p class="text-sm text-gray-500">Loading...</p>
		</div>
	</div>
{:else if db.isOpen}
	{@render children()}
{:else}
	<div class="flex min-h-screen items-center justify-center bg-gradient-to-br from-primary-50 via-white to-primary-100">
		<div class="mx-4 w-full max-w-md rounded-2xl bg-white p-8 shadow-xl">
			<div class="mb-6 text-center">
				<div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-xl bg-primary-600 text-2xl font-bold text-white">
					IM
				</div>
				<h1 class="text-2xl font-bold text-gray-900">Invoice Manager</h1>
				<p class="mt-2 text-sm text-gray-500">
					A local-first invoice management tool. Your data stays on your device, stored in a SQLite database file you control.
				</p>
			</div>

			<div class="space-y-3">
				{#if storedFileName}
					<button
						onclick={() => reconnect().then(ok => { if (!ok) storedFileName = null; })}
						class="w-full cursor-pointer rounded-lg bg-primary-600 px-4 py-3 font-medium text-white transition-colors hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					>
						Reconnect to {storedFileName}
					</button>
					<div class="relative my-2">
						<div class="absolute inset-0 flex items-center"><div class="w-full border-t border-gray-200"></div></div>
						<div class="relative flex justify-center"><span class="bg-white px-3 text-xs text-gray-400">or</span></div>
					</div>
				{/if}
				<button
					onclick={initNew}
					class="w-full cursor-pointer rounded-lg {storedFileName ? 'border border-gray-300 bg-white text-gray-700 hover:bg-gray-50' : 'bg-primary-600 text-white hover:bg-primary-700'} px-4 py-3 font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				>
					Create New Database
				</button>
				<button
					onclick={openExisting}
					class="w-full cursor-pointer rounded-lg border border-gray-300 bg-white px-4 py-3 font-medium text-gray-700 transition-colors hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				>
					Open Existing Database
				</button>
			</div>

			<p class="mt-6 text-center text-xs text-gray-400">
				Files are saved locally using the File System Access API.
			</p>
		</div>
	</div>
{/if}
