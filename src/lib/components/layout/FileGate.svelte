<script lang="ts">
	import type { Snippet } from 'svelte';
	import { db, initNew, openExisting } from '$lib/db/connection.svelte';

	let { children }: { children: Snippet } = $props();
</script>

{#if db.isOpen}
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
				<button
					onclick={initNew}
					class="w-full cursor-pointer rounded-lg bg-primary-600 px-4 py-3 font-medium text-white transition-colors hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
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
