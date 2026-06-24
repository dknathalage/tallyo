<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiPost } from '$lib/api/client';
	import { session } from '$lib/stores/session.svelte';
	import type { User } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';
	import Receipt from '@lucide/svelte/icons/receipt';

	let businessName = $state('');
	let name = $state('');
	let email = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const user = await apiPost<User>('/api/signup', {
				businessName,
				name,
				email,
				password
			});
			// Signup establishes the session; land logged in on the new tenant.
			session.set(user);
			const info = await session.loadSession();
			const first = info?.tenants[0];
			await goto(first ? '/' + first.id + '/' : '/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Sign up failed.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="mx-auto flex min-h-screen max-w-sm flex-col justify-center px-4 py-12">
	<a href="/login" class="mb-6 flex items-center justify-center gap-2">
		<span class="flex size-8 items-center justify-center rounded-lg bg-brand-700 text-onbrand">
			<Receipt class="size-5" aria-hidden="true" />
		</span>
		<span class="text-xl font-semibold tracking-tight text-brand-700">Tallyo</span>
	</a>

	<Card>
		<h1 class="mb-1 text-xl font-semibold tracking-tight">Create your Tallyo account</h1>
		<p class="mb-6 text-sm text-gray-500">Set up your business in one step.</p>

		<form class="space-y-4" onsubmit={submit}>
			<Field label="Business name" id="businessName">
				<input
					id="businessName"
					type="text"
					bind:value={businessName}
					required
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</Field>
			<Field label="Your name" id="name">
				<input
					id="name"
					type="text"
					bind:value={name}
					required
					autocomplete="name"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</Field>
			<Field label="Email" id="email">
				<input
					id="email"
					type="email"
					bind:value={email}
					required
					autocomplete="email"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</Field>
			<Field label="Password" id="password" hint="At least 8 characters.">
				<input
					id="password"
					type="password"
					bind:value={password}
					required
					minlength="8"
					autocomplete="new-password"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</Field>
			{#if error}
				<p class="text-sm text-red-600" role="alert">{error}</p>
			{/if}

			<Button type="submit" loading={submitting} class="w-full">Create account</Button>
		</form>

		<p class="mt-4 text-center text-sm text-gray-500">
			Already have an account? <a href="/login" class="font-medium text-brand-700 hover:text-brand-800"
				>Sign in</a
			>
		</p>
	</Card>
</div>
